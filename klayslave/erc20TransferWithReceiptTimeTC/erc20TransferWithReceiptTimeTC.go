package erc20TransferWithReceiptTimeTC

import (
	"context"
	"fmt"
	client "github.com/ethereum/go-ethereum/ethclient"
	"github.com/klaytn/klaytn-load-tester/klayslave/account"
	"github.com/klaytn/klaytn-load-tester/klayslave/clipool"
	"github.com/klaytn/klaytn-load-tester/klayslave/task"
	"log"
	"math/big"
	"math/rand"
	"sync"
	"time"

	"github.com/myzhan/boomer"
)

const Name = "erc20TransferTCWithReceiptTime"

var (
	endPoint string
	nAcc     int
	accGrp   []*account.Account
	cliPool  clipool.ClientPool
	gasPrice *big.Int

	// multinode tester
	transferedValue *big.Int
	expectedFee     *big.Int

	fromAccount     *account.Account
	prevBalanceFrom *big.Int

	toAccount     *account.Account
	prevBalanceTo *big.Int

	SmartContractAccount *account.Account

	ctx            context.Context
	cancel         context.CancelFunc
	running        sync.WaitGroup
	currentBlock   int64
	blockReadTimes map[int64]int64
	txMap          map[string]int64
	txMu           sync.Mutex
)

func Init(params task.Params) {
	gasPrice = params.GasPrice

	endPoint = params.Endpoint

	cliCreate := func() interface{} {
		c, err := client.Dial(endPoint)
		if err != nil {
			log.Fatalf("Failed to connect RPC: %v", err)
		}
		return c
	}

	cliPool.Init(20, 300, cliCreate)

	for _, acc := range params.AccGrp {
		accGrp = append(accGrp, acc)
	}

	nAcc = len(accGrp)

	ctx, cancel = context.WithCancel(context.Background())
	running = sync.WaitGroup{}
	currentBlock = -1
	blockReadTimes = make(map[int64]int64)
	txMap = make(map[string]int64)
	//txMetaMap = make(map[string]txMeta)
	//groups = make(map[int][]string)
	//groupToMove = 0
	txMu = sync.Mutex{}
	running.Add(1)
	go loopReceiptCheck()
}

type txMeta struct {
	sent  int64
	group int
}

func Run() {
	cli := cliPool.Alloc().(*client.Client)

	from := accGrp[rand.Int()%nAcc]
	to := accGrp[rand.Int()%nAcc]
	value := big.NewInt(int64(rand.Int() % 3))

	start := time.Now()
	tx, _, err := from.TransferERC20(false, cli, SmartContractAccount.GetAddress(), to, value)
	if err != nil {
		elapsed := time.Since(start)
		elapsedMillis := elapsed.Milliseconds()
		if elapsedMillis == 0 {
			fmt.Printf("found zero elapsed ms. %d ns\n", elapsed.Nanoseconds())
		}

		boomer.RecordFailure("http", Name+"_fail", elapsedMillis, err.Error())
	}
	pushTransaction(tx.Hash().Hex(), start.UnixNano()/int64(time.Millisecond))
}

func Stop() {
	cancel()
	running.Wait()
}

func pushTransaction(hash string, requestTime int64) {
	txMu.Lock()
	txMap[hash] = requestTime
	txMu.Unlock()
}

func loopReceiptCheck() {
	interval := time.Millisecond * 100
	timer := time.NewTimer(interval)
	defer func() {
		running.Done()
		timer.Stop()
	}()

	for {
		select {
		case <-timer.C:
			checkBlock()
			timer.Reset(interval)
		case <-ctx.Done():
			return
		}
	}
}

func checkBlock() {
	cli := cliPool.Alloc().(*client.Client)
	defer cliPool.Free(cli)

	if currentBlock <= 0 {
		bn, err := cli.BlockNumber(ctx)
		if err != nil {
			return
		}
		currentBlock = int64(bn) - 1
	}

	blockNumber := big.NewInt(currentBlock + 1)
	block, err := cli.BlockByNumber(ctx, blockNumber)
	if err != nil {
		return
	}

	readTime := boomer.Now()
	running.Add(1)
	go func() {
		defer running.Done()

		for _, tx := range block.Transactions() {
			hash := tx.Hash().Hex()
			txMu.Lock()
			requestTime, ok := txMap[hash]
			txMu.Unlock()
			if !ok {
				continue
			}
			elapsed := readTime - requestTime
			boomer.RecordSuccess("http", Name, elapsed, int64(10))
			txMu.Lock()
			delete(txMap, hash)
			txMu.Unlock()
		}
	}()
	log.Printf("Read block: %d, txs: %d, readTime: %d, txs: %d", block.NumberU64(), len(block.Transactions()), readTime, len(txMap))

	blockReadTimes[blockNumber.Int64()] = readTime
	currentBlock++
}
