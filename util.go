package main

import (
	"fmt"
	"github.com/ququzone/ckb-sdk-go/types"
	"log"
	"github.com/ququzone/ckb-sdk-go/crypto/secp256k1"
	"github.com/ququzone/ckb-sdk-go/transaction"
	"github.com/ququzone/ckb-sdk-go/utils"
	"github.com/ququzone/ckb-sdk-go/rpc"
)

func GenTxAndCollectCells(client rpc.Client, key *secp256k1.Secp256k1Key, collectAmount uint64) (*types.Transaction, *utils.SystemScripts, *types.Script, *utils.CollectResult, error) {
	systemScripts, err := utils.NewSystemScripts(client)
	if err != nil {
		log.Fatalf("load system script error: %v", err)
	}
	lockScript, err := key.Script(systemScripts)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// collect utxo cells
	collector := utils.NewCellCollector(client, lockScript, utils.NewCapacityCellProcessor(collectAmount))
	result, err := collector.Collect()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("collect cell error: %v", err)
	}
	if result.Capacity < CellCapacity+Fee {
		return nil, nil, nil, nil, fmt.Errorf("insufficient balance: %d", result.Capacity)
	}

	tx := transaction.NewSecp256k1SingleSigTx(systemScripts)
	return tx, systemScripts, lockScript, result, nil
}
