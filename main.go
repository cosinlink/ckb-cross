package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/ququzone/ckb-sdk-go/crypto/blake2b"
	"io/ioutil"
	"log"
	"time"

	"github.com/ququzone/ckb-sdk-go/crypto/secp256k1"
	"github.com/ququzone/ckb-sdk-go/rpc"
	"github.com/ququzone/ckb-sdk-go/transaction"
	"github.com/ququzone/ckb-sdk-go/types"
	"github.com/ququzone/ckb-sdk-go/utils"
)

const Fee = 100000000
const privateKey = "d00c06bfd800d27397002dca6fb0993d5ba6399b4238b2f29ee9deb97593d2bc"
const bPrivKey = "d00c06bfd800d27397002dca6fb0993d5ba6399b4238b2f29ee9deb97593d2b0"
const SimpleUdtFilePath = "./deps/simple_udt"
const M2CTypeScriptFilePath = "./deps/always_success"
const C2MLockScriptFilePath = "./deps/crosschain_lockscript"

type Config struct {
	SimpleUdtBinary []byte
	SimpleUdtHash   types.Hash

	M2CTypeScriptBinary []byte
	M2CTypeScriptHash   types.Hash

	C2MLockScriptBinary []byte
	C2MLockScriptHash   types.Hash

	CodeTxHash types.Hash
}

func main() {
	client, err := rpc.Dial("http://127.0.0.1:8114")
	if err != nil {
		log.Fatalf("create rpc client error: %v", err)
	}

	config, err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	err = Deploy(config, client, privateKey, config.SimpleUdtBinary, config.M2CTypeScriptBinary, config.C2MLockScriptBinary)
	if err != nil {
		log.Fatal(err)
	}

	err = waitForTx(client, config.CodeTxHash)
	if err != nil {
		log.Fatal(err)
	}

	err = IssueSudt(config, client, bPrivKey)
	if err != nil {
		log.Fatal(err)
	}
}

func waitForTx(client rpc.Client, txHash types.Hash) error {
	log.Println("wait for tx: ", txHash.Hex())
	for {
		txStatus, err := client.GetTransaction(context.Background(), txHash)
		if err != nil {
			return err
		}

		log.Println("tx status: ", txStatus.TxStatus.Status)
		if txStatus.TxStatus.Status == types.TransactionStatusCommitted {
			break
		}

		time.Sleep(time.Second)
	}
	return nil
}

func Deploy(config *Config, client rpc.Client, hexKey string, codeList ...[]byte) error {
	key, err := secp256k1.HexToKey(hexKey)
	if err != nil {
		return err
	}
	systemScripts, err := utils.NewSystemScripts(client)
	if err != nil {
		log.Fatalf("load system script error: %v", err)
	}
	lockScript, err := key.Script(systemScripts)
	if err != nil {
		return err
	}

	var capacitySum uint64 = 0
	capList := make([]uint64, len(codeList))
	for i, data := range codeList {
		capList[i] = uint64(len(data))*100000000 + 4100000000
		capacitySum += capList[i]
	}

	// collect utxo cells
	collector := utils.NewCellCollector(client, lockScript, utils.NewCapacityCellProcessor(capacitySum+Fee))
	result, err := collector.Collect()
	if err != nil {
		return fmt.Errorf("collect cell error: %v", err)
	}
	if result.Capacity < capacitySum+Fee {
		return fmt.Errorf("insufficient balance: %d", result.Capacity)
	}

	tx := transaction.NewSecp256k1SingleSigTx(systemScripts)
	group, witnessArgs, err := transaction.AddInputsForTransaction(tx, result.Cells)
	if err != nil {
		return fmt.Errorf("add inputs to transaction error: %v", err)
	}

	for i, data := range codeList {
		tx.Outputs = append(tx.Outputs, &types.CellOutput{
			Capacity: capList[i],
			Lock: &types.Script{
				HashType: types.HashTypeData,
			},
		})
		tx.OutputsData = append(tx.OutputsData, data)
	}

	// change
	tx.Outputs = append(tx.Outputs, &types.CellOutput{
		Capacity: result.Capacity - capacitySum - Fee,
		Lock:     lockScript,
	})
	tx.OutputsData = append(tx.OutputsData, []byte{})

	err = transaction.SingleSignTransaction(tx, group, witnessArgs, key)
	if err != nil {
		return fmt.Errorf("sign transaction error: %v", err)
	}

	txHash, err := client.SendTransaction(context.Background(), tx)
	if err != nil {
		return fmt.Errorf("SendTransaction error: %v", err)
	}

	config.CodeTxHash = *txHash
	log.Println("SendTransaction Deploy: ", hex.EncodeToString(txHash[:]))
	return nil
}

func IssueSudt(config *Config, client rpc.Client, hexKey string) error {
	adminKey, err := secp256k1.HexToKey(privateKey)
	if err != nil {
		return err
	}
	userKey, err := secp256k1.HexToKey(hexKey)
	if err != nil {
		return err
	}

	systemScripts, err := utils.NewSystemScripts(client)
	if err != nil {
		log.Fatalf("load system script error: %v", err)
	}

	adminLockScript, err := adminKey.Script(systemScripts)
	if err != nil {
		return err
	}

	adminLockScriptHash, err := adminLockScript.Hash()
	if err != nil {
		return err
	}

	// cost capacity of sudt cell
	const CellCapacity = 20000000000000

	// collect utxo cells
	collector := utils.NewCellCollector(client, adminLockScript, utils.NewCapacityCellProcessor(CellCapacity+Fee))
	result, err := collector.Collect()
	if err != nil {
		return fmt.Errorf("collect cell error: %v", err)
	}
	if result.Capacity < CellCapacity+Fee {
		return fmt.Errorf("insufficient balance: %d", result.Capacity)
	}

	userLockScript, err := userKey.Script(systemScripts)
	if err != nil {
		return err
	}

	tx := transaction.NewSecp256k1SingleSigTx(systemScripts)

	// add sudt code cell into cellDeps
	tx.CellDeps = append(tx.CellDeps, &types.CellDep{
		OutPoint: &types.OutPoint{
			TxHash: config.CodeTxHash,
			Index:  0,
		},
		DepType: types.DepTypeCode,
	})

	// admin will pay the cost and receive the change
	group, witnessArgs, err := transaction.AddInputsForTransaction(tx, result.Cells)
	if err != nil {
		return fmt.Errorf("add inputs to transaction error: %v", err)
	}

	// sudt cell
	tx.Outputs = append(tx.Outputs,
		&types.CellOutput{
			Capacity: CellCapacity,
			Lock:     userLockScript,
			Type: &types.Script{
				CodeHash: config.SimpleUdtHash,
				HashType: types.HashTypeData,
				Args:     adminLockScriptHash[:],
			},
		})

	// change
	tx.Outputs = append(tx.Outputs,
		&types.CellOutput{
			Capacity: result.Capacity - CellCapacity - Fee,
			Lock:     adminLockScript,
		})

	// set OutputsData
	var sudtNum uint32 = 6543421
	tx.OutputsData = [][]byte{types.SerializeUint(uint(sudtNum)), {}}

	err = transaction.SingleSignTransaction(tx, group, witnessArgs, adminKey)
	if err != nil {
		return fmt.Errorf("sign transaction error: %v", err)
	}

	txHash, err := client.SendTransaction(context.Background(), tx)
	if err != nil {
		return fmt.Errorf("SendTransaction error: %v", err)
	}

	log.Println("SendTransaction Issue Sudt: ", hex.EncodeToString(txHash[:]))
	return nil
}

func LoadConfig() (*Config, error) {
	data, err := ioutil.ReadFile(SimpleUdtFilePath)
	if err != nil {
		return nil, err
	}
	hash, err := blake2b.Blake256(data)
	if err != nil {
		return nil, err
	}

	dataM2CType, err := ioutil.ReadFile(M2CTypeScriptFilePath)
	if err != nil {
		return nil, err
	}
	hashM2CType, err := blake2b.Blake256(dataM2CType)
	if err != nil {
		return nil, err
	}

	dataC2MLock, err := ioutil.ReadFile(C2MLockScriptFilePath)
	if err != nil {
		return nil, err
	}
	hashC2MLock, err := blake2b.Blake256(dataC2MLock)
	if err != nil {
		return nil, err
	}

	config := &Config{
		SimpleUdtBinary: data,
		SimpleUdtHash:   types.BytesToHash(hash),

		M2CTypeScriptBinary: dataM2CType,
		M2CTypeScriptHash:   types.BytesToHash(hashM2CType),

		C2MLockScriptBinary: dataC2MLock,
		C2MLockScriptHash:   types.BytesToHash(hashC2MLock),
	}

	return config, nil
}
