package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/ququzone/ckb-sdk-go/crypto/blake2b"
	"io/ioutil"
	"log"

	"github.com/ququzone/ckb-sdk-go/crypto/secp256k1"
	"github.com/ququzone/ckb-sdk-go/rpc"
	"github.com/ququzone/ckb-sdk-go/transaction"
	"github.com/ququzone/ckb-sdk-go/types"
	"github.com/ququzone/ckb-sdk-go/utils"
)

const privateKey = "d00c06bfd800d27397002dca6fb0993d5ba6399b4238b2f29ee9deb97593d2bc"
const bPrivKey = "d00c06bfd800d27397002dca6fb0993d5ba6399b4238b2f29ee9deb97593d2b0"
const SimpleUdtFilePath = "./deps/simple_udt"

type Config struct {
	SimpleUdtBinary []byte
	SimpleUdtHash   types.Hash
}

func main() {
	client, err := rpc.Dial("http://127.0.0.1:8114")
	if err != nil {
		log.Fatalf("create rpc client error: %v", err)
	}

	err = IssueSudt(client, bPrivKey)
	if err != nil {
		log.Fatal(err)
	}
}

func IssueSudt(client rpc.Client, hexKey string) error {
	config, err := LoadConfig()
	if err != nil {
		return err
	}

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

	// cost capacity of sudt cell
	const CellCapacity = 20000000000000
	const Fee = 100000000

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

	// admin will pay the cost and receive the change
	group, witnessArgs, err := transaction.AddInputsForTransaction(tx, result.Cells)
	if err != nil {
		return fmt.Errorf("add inputs to transaction error: %v", err)
	}

	// sudt cell
	newSudtArgs, err := tx.Inputs[0].Serialize()
	if err != nil {
		return err
	}
	tx.Outputs = append(tx.Outputs,
		&types.CellOutput{
			Capacity: CellCapacity,
			Lock:     userLockScript,
			Type: &types.Script{
				CodeHash: config.SimpleUdtHash,
				HashType: types.HashTypeData,
				Args:     newSudtArgs,
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

	log.Println("SendTransaction: ", hex.EncodeToString(txHash[:]))
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
	config := &Config{
		SimpleUdtBinary: data,
		SimpleUdtHash:   types.BytesToHash(hash),
	}
	return config, nil
}
