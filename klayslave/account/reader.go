package account

import (
	"bufio"
	"errors"
	"io"
	"log"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
)

var ErrNoData = errors.New("no data")

// Reader represents account generator
type Reader interface {
	Read(count int) ([]*Account, error)
	Remain() int
}

type reader struct{}

// NewReader creates a new Reader which generate accounts each time
func NewReader() Reader {
	return &reader{}
}

func (r *reader) Read(count int) ([]*Account, error) {
	if count <= 0 {
		return []*Account{}, nil
	}
	accounts := make([]*Account, count)
	workers := runtime.NumCPU()
	accCountPerWorker := count / workers
	accountIdx := int32(-1)
	wg := sync.WaitGroup{}

	for i := 0; i < workers; i++ {
		accCountPerWorker := accCountPerWorker
		modulo := count % workers
		if modulo != 0 && i < modulo {
			accCountPerWorker++
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < accCountPerWorker; i++ {
				idx := atomic.AddInt32(&accountIdx, 1)
				accounts[idx] = NewAccount(int(idx))
			}
		}()
	}

	wg.Wait()
	return accounts, nil
}

func (r *reader) Remain() int {
	return -1
}

type fileReader struct {
	accounts        []*Account
	generator       Reader
	generateIfEmpty bool
}

// NewFileReader creates a new Reader which read private keys from given paths.
func NewFileReader(paths []string, generateIfEmpty bool) (Reader, error) {
	var (
		accounts []*Account
		errs     []error
		mtx      sync.Mutex
		wg       sync.WaitGroup
	)

	for _, path := range paths {
		wg.Add(1)
		go func(filepath string) {
			log.Printf("try to read key file: %s", filepath)
			defer wg.Done()
			read, err := readAccount(filepath)
			mtx.Lock()
			if err == nil {
				accounts = append(accounts, read...)
			} else {
				errs = append(errs, err)
			}
			mtx.Unlock()
		}(path)
	}
	wg.Wait()

	if len(errs) != 0 {
		return nil, errs[0]
	}

	log.Printf("Initialize account file reader. paths: %d, read account: %d", len(paths), len(accounts))

	return &fileReader{
		accounts:        accounts,
		generator:       &reader{},
		generateIfEmpty: generateIfEmpty,
	}, nil
}

func (r *fileReader) Read(count int) ([]*Account, error) {
	if count <= 0 {
		return []*Account{}, nil
	}
	end := count
	if end > len(r.accounts) {
		end = len(r.accounts)
	}

	results := r.accounts[:end]
	remain := count - len(results)
	if remain == 0 {
		r.accounts = r.accounts[end:]
		return results, nil
	}
	if !r.generateIfEmpty {
		return nil, ErrNoData
	}
	generated, err := r.generator.Read(remain)
	if err != nil {
		return nil, err
	}
	results = append(results, generated...)
	r.accounts = r.accounts[end:]
	return results, nil
}

func (r *fileReader) Remain() int {
	return len(r.accounts)
}

func readAccount(filepath string) ([]*Account, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var accounts []*Account
	buf := bufio.NewReader(f)
	for {
		line, _, err := buf.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if len(line) == 0 {
			continue
		}
		acc := GetAccountFromKey(0, string(line))
		accounts = append(accounts, acc)
	}
	return accounts, nil
}
