# Branch name will be changed

We will change the `master` branch to `main` on Nov 1, 2022.
After the branch policy change, please check your local or forked repository settings.

# klaytn-load-tester
klaytn-load-tester is a load tester using boomer library and it is written in golang.

It provides built-in test cases that run to the klaytn node. It can spawn thousands of goroutines to run your test code concurrently.
It will listen and report to the locust master automatically, and your test results will be displayed on the master's web UI.
* Locust Website: <a href="http://locust.io">locust.io</a>
* Locust Documentation: <a href="http://docs.locust.io">docs.locust.io</a>

## Prerequisite
Clone the klaytn-load-tester Repository. 

Go should be installed, then build klayslave/main.go. It will be used to run locust slave.
```bash
$ cd klaytn-load-tester
$ make build
$ ./build/bin/klayslave version
```
To test ethereum tx types via locust, you should update git submodule
```bash
$ git submodule init && git submodule update
```
Install locust 1.2.3 in python3 virtual environment. It will be used to run locust master.
```bash
$ cd klaytn-load-tester
$ python3 -m venv venv
$ source venv/bin/activate
(venv)$ pip3 install locust==1.2.3
```

Provide a klaytn node rpc endpoint and rich account.
* klaytn node rpc endpoint: it will receive the requests generated by locust.
* rich account: a test account which has a lots of money

## Quick Start

Locust master: Run locust master. 

Open new terminal, then enter next script.
```bash
(venv)$ locust -f dist/locustfile.py --master
```

Locust slave: You can run one or more test cases as follow
* KEY: private key of rich account
  * example: 349343aad78f398528907e62b62ce7e7e3c9f57c674e12bbd03857682334a73f
* ENDPOINT: klaytn node rpc endpoint
  * example: "http://localhost:8551"

Open new terminal, then enter next script with rich account key and your klaytn node rpc endpoint.
```bash
$ ./build/bin/klayslave --max-rps 150 --master-host localhost --master-port 5557 -key $KEY \
                                -tc="transferSignedTx"  -endpoint $ENDPOINT
```

Start a load test
* To start the test, enter the locust master web UI
* Enter Number of User, Hatch Rate. For example, enter 100, 100.
A higher value is better if the TC get a quick response (e.g. readApiCallTC).
* When a slave is ready, the status shows `1 users`. If there are more slaves, more users will be shown.
* Click `Start swarming`, then you can see the results of the requests in real time.

Parameters
* --max-rps: limit of request per second
* --master-host: master host ip address
* --master-port: master port number
* --key: private key to fund to internal klay test accounts that created before run test case. This creates keystore file on to the target klay node.
* --vusigned : number of accounts for signed transaction to use in test case.
* --vuunsigned: number of accounts for unsigned transaction to use in test case.
* --endpoint: klay node rpc endpoint(e.g. http://localhost:8551).

## How to contribute?
* issue: Please make an issue if there's bug, improvement, docs suggestion, etc.
* contribute: Please make a PR. If the PR is related with an issue, link the issue.

## Build and run with docker
(deprecated) this will be fixed
```bash
$ git clone https://github.com/klaytn/klaytn
$ cd klaytn && go mod vendor && cd ..
$ dockertag=locust-$(date +%s)
$ docker build . -t $dockertag
$ docker run --rm -v $(pwd):/tmp1 $dockertag bash -c 'cp -r klaytn-docker-pkg/* /tmp1'
```

## Definition of User, Hatch rate, RPS
* **Number of User**: The sum of the number of the GoRoutines created by all slaves. 
The goroutines are equally allocated to each slave. 
That is, if there are 10 users and 2 slaves, 10 total GoRoutines are created, and 5 slaves are allocated to each slave.
* **Hatch Rate** (users hatched/second): The speed to reach the target number of GoRoutines(User).
If the number of user is 100 and hatch rate is 10, GoRoutine is created for 10 (=100/10) seconds.
* **RPS** (Request per second): The degree of load requests to the all endpoints. 
RPS generated per slave is RPS/number-of-slave. The value of --max-rps is the target value that we try to make to the maximum. 

Reference code: https://github.com/myzhan/boomer/blob/master/runner.go spawnWorkers

## Troubleshooting
* check that klaytn network block producing is not blocked.
* check master or slave log
  * the HTTP connection fails: Check whether slave and node are connectable networks
    * e.g. Failed to connect RPC ...
  * the provided private key value is incorrect
    * e.g. Failed to HexToECDSA, Failed to Unlock
  * the balance of the test account is not enough: attach to the endpoint, and check the balance
  * If it is output as Method that is not in RPC
    * check RPC_ENABLE and RPC_API option at kcnd.conf/kpnd.conf/kend.conf
    * RPC_ENABLE should be 1
    * RPC_API should contain txpool,klay,net,web3,miner,personal,admin,rpc

## License
Open source licensed under the MIT license (see [_LICENSE_](LICENSE) file for details).
