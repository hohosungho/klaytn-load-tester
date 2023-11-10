package receiptChecker

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	client "github.com/ethereum/go-ethereum/ethclient"
	"github.com/klaytn/klaytn-load-tester/klayslave/clipool"
	"github.com/myzhan/boomer"
	"log"
	"math/rand"
	"time"
)

type ReceiptChecker interface {
	PushTransaction(hash string)
	Check()
}

type receiptChecker struct {
	useSampling   bool
	samplingRatio int // 1:max-rps
	sampler       int
	ctx           context.Context
	hashChan      chan string
	tcName        string
	endpoint      string
	cliPool       *clipool.ClientPool
}

func NewDefaultReceiptChecker(ctx context.Context, maxRPS int, tcName, endpoint string) ReceiptChecker {
	cliCreate := func() interface{} {
		c, err := client.Dial(endpoint)
		if err != nil {
			log.Fatalf("Failed to connect RPC: %v", err)
		}
		return c
	}

	cliPool := &clipool.ClientPool{}
	cliPool.Init(20, 300, cliCreate)

	log.Printf("Initialized Receipt Checker for TC: (%s) with RPS: (%d)\n", tcName, maxRPS)
	return &receiptChecker{
		useSampling:   true,
		samplingRatio: maxRPS,
		sampler:       rand.Intn(maxRPS),
		ctx:           ctx,
		hashChan:      make(chan string),
		tcName:        tcName,
		endpoint:      endpoint,
		cliPool:       cliPool,
	}
}

func (r *receiptChecker) PushTransaction(hash string) {
	if r.useSampling {
		n := rand.Intn(r.samplingRatio)
		if n == r.sampler {
			r.hashChan <- hash
		}
	} else {
		r.hashChan <- hash
	}
}

func (r *receiptChecker) Check() {
	cli := r.cliPool.Alloc().(*client.Client)
	defer r.cliPool.Free(cli)

	for {
		select {
		case hash := <-r.hashChan:
			go func() {
				queryTicker := time.NewTicker(time.Millisecond * 500)
				readTime := boomer.Now()
				for i := 0; i < 50; i++ { // 25초
					receipt, err := cli.TransactionReceipt(r.ctx, common.HexToHash(hash))
					if receipt != nil {
						boomer.RecordSuccess("http", r.tcName+" "+r.endpoint, boomer.Now()-readTime, int64(10))
						queryTicker.Stop()
						break
					}
					if err != nil {
						// Receipt retrieval failed
					} else {
						// Transaction not yet mined
					}

					select {
					case <-r.ctx.Done():
						return
					case <-queryTicker.C:
					}
				}
				log.Printf("Timed out: hash(%s)\n", hash)
			}()
		case <-r.ctx.Done():
			return
		}
	}
}
