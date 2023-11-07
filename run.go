// Copyright (c) 2022 The illium developers
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/project-illium/ilxd/repo"
	"github.com/project-illium/ilxd/rpc/pb"
	"github.com/project-illium/ilxd/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"math/rand"
	"time"
)

func run(config *SwarmConfig, done chan struct{}) {
	var (
		genesisWalletClient pb.WalletServiceClient
		chainClients        []pb.BlockchainServiceClient
		addrChan            = make(chan string)
		workChans           = make([]chan struct{}, len(config.Nodes))
		err                 error
	)
	genesisWalletClient, err = makeWalletClient(config.GenesisNode, config.RPCCert)
	if err != nil {
		log.Fatalf("Error creating connection to genesis node: %s", err)
	}
	genesisChainClient, err := makeBlockchainClient(config.GenesisNode, config.RPCCert)
	if err != nil {
		log.Fatalf("Error creating connection to genesis node: %s", err)
	}
	chainClients = append(chainClients, genesisChainClient)

	for i, n := range config.Nodes {
		chainClient, err := makeBlockchainClient(n, config.RPCCert)
		if err != nil {
			log.Fatalf("Error creating connection to node: %s", err)
		}
		chainClients = append(chainClients, chainClient)
		walletClient, err := makeWalletClient(n, config.RPCCert)
		if err != nil {
			log.Fatalf("Error creating connection to genesis node: %s", err)
		}
		workChans[i] = make(chan struct{})
		go worker(walletClient, addrChan, workChans[i], done)
	}

	var totalCoins uint64
	resp, err := genesisWalletClient.GetUtxos(context.Background(), &pb.GetUtxosRequest{})
	if err != nil {
		log.Fatalf("Error get genesis utxos: %s", err)
	}
	for _, u := range resp.Utxos {
		if !u.Staked {
			totalCoins += u.Amount
		}
	}

	amts := distributeStakes(totalCoins, len(config.Nodes))

	i := 0
	go func() {
		start := time.Now()
		workTicker := time.NewTicker(time.Minute / time.Duration(config.TxPerMinute))
		printTicker := time.NewTicker(time.Minute)
		for {
			select {
			case <-done:
				return
			case addr := <-addrChan:
				if i >= len(config.Nodes) {
					continue
				}
				sub, err := genesisWalletClient.SubscribeWalletTransactions(context.Background(), &pb.SubscribeWalletTransactionsRequest{})
				if err != nil {
					log.Errorf("Error subscribing to block stream: %s", err)
					continue
				}
				resp, err := genesisWalletClient.Spend(context.Background(), &pb.SpendRequest{
					ToAddress: addr,
					Amount:    amts[i] - 1000000,
				})
				if err != nil {
					log.Errorf("Error spending coins: %s", err)
					continue
				}

				if err := waitForFinalization(sub, types.NewID(resp.Transaction_ID), done); err != nil {
					log.Errorf("Sending initial coins failed: %s", err)
				}
				i++
			case <-workTicker.C:
				if time.Now().Before(start.Add(time.Second * 10)) {
					continue
				}
				r := rand.Intn(len(config.Nodes))
				workChans[r] <- struct{}{}
			case <-printTicker.C:
				m := make(map[uint32]int)
				for _, c := range chainClients {
					resp, err := c.GetBlockchainInfo(context.Background(), &pb.GetBlockchainInfoRequest{})
					if err != nil {
						log.Errorf("Error querying for block height: %s", err)
						continue
					}
					m[resp.BestHeight]++
				}
				for k, v := range m {
					log.Infof("Status: %d nodes at height %d", v, k)
				}
			}
		}
	}()
}

func worker(walletClient pb.WalletServiceClient, addrChan chan string, workChan chan struct{}, done chan struct{}) {
	resp, err := walletClient.GetAddress(context.Background(), &pb.GetAddressRequest{})
	if err != nil {
		log.Errorf("Error fetching wallet address: %s", err)
		return
	}
	sub, err := walletClient.SubscribeWalletTransactions(context.Background(), &pb.SubscribeWalletTransactionsRequest{})
	if err != nil {
		log.Errorf("Error fetching wallet address: %s", err)
		return
	}

	addrChan <- resp.Address

	if err := waitForFinalization(sub, types.ID{}, done); err != nil {
		log.Errorf("Error receiving initial coins: %s", err)
		return
	}

	sub, err = walletClient.SubscribeWalletTransactions(context.Background(), &pb.SubscribeWalletTransactionsRequest{})
	if err != nil {
		log.Errorf("Error subscribing to block stream: %s", err)
		return
	}
	spendingMoney := uint64(100000000000)
	spendResp, err := walletClient.Spend(context.Background(), &pb.SpendRequest{
		ToAddress: resp.Address,
		Amount:    spendingMoney,
	})

	if err := waitForFinalization(sub, types.NewID(spendResp.Transaction_ID), done); err != nil {
		log.Errorf("Sending initial coins failed: %s", err)
		return
	}

	utxoResp, err := walletClient.GetUtxos(context.Background(), &pb.GetUtxosRequest{})
	if err != nil {
		log.Errorf("Error get genesis utxos: %s", err)
	}
	var utxo *pb.Utxo
	for _, u := range utxoResp.Utxos {
		if u.Amount != spendingMoney {
			utxo = u
			break
		}
	}
	if utxo == nil {
		log.Error("Failed to load stake utxo")
		return
	}

	_, err = walletClient.Stake(context.Background(), &pb.StakeRequest{
		Commitments: [][]byte{utxo.Commitment},
	})

	for {
		select {
		case <-done:
			return
		case <-workChan:
			sub, err = walletClient.SubscribeWalletTransactions(context.Background(), &pb.SubscribeWalletTransactionsRequest{})
			if err != nil {
				log.Errorf("Error subscribing to block stream: %s", err)
				return
			}
			spendResp, err = walletClient.Spend(context.Background(), &pb.SpendRequest{
				ToAddress: resp.Address,
				Amount:    0,
			})

			if err := waitForFinalization(sub, types.NewID(spendResp.Transaction_ID), done); err != nil {
				log.Errorf("Sending initial coins failed: %s", err)
				return
			}
		}
	}
}

func waitForFinalization(sub pb.WalletService_SubscribeWalletTransactionsClient, txid types.ID, done chan struct{}) error {
	out := make(chan error)
	defer close(out)
	defer sub.CloseSend()

	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			notif, err := sub.Recv()
			if err != nil {
				out <- err
				return
			}
			id := types.NewID(notif.Transaction.Transaction_ID)
			if id.Compare(txid) == 0 || txid.Compare(types.ID{}) == 0 {
				out <- nil
				return
			}
		}
	}()

	select {
	case <-done:
		return nil
	case err := <-out:
		return err
	case <-time.After(time.Second * 5):
		return errors.New("timed out waiting for tx validation")
	}
}

func distributeStakes(totalCoins uint64, nodes int) []uint64 {
	// Initialize the seed for the random number generator
	rand.Seed(time.Now().UnixNano())

	// Initialize a slice to hold the stakes for each node
	stakes := make([]uint64, nodes)
	var sumStakes uint64

	// Calculate the initial stake per node to be within a safe range
	initialStake := totalCoins / uint64(nodes)

	for i := 0; i < nodes; i++ {
		// Assign the initial stake to each node
		stakes[i] = initialStake
		sumStakes += initialStake
	}

	// Randomly distribute the remaining coins
	remaining := totalCoins - sumStakes
	for remaining > 0 {
		// Pick a random node to receive an extra coin
		node := rand.Intn(nodes)
		stakes[node]++
		remaining--
		sumStakes++
	}

	// Ensure that the stakes do not exceed the total coins by reducing them if necessary
	for sumStakes > totalCoins {
		// Pick a random node to reduce its stake
		node := rand.Intn(nodes)
		// Only reduce the stake if it's greater than 1 to maintain a fair distribution
		if stakes[node] > 1 {
			stakes[node]--
			sumStakes--
		}
	}

	return stakes
}

func makeBlockchainClient(serverAddr string, rpcCertFile string) (pb.BlockchainServiceClient, error) {
	certFile := repo.CleanAndExpandPath(rpcCertFile)

	var (
		creds credentials.TransportCredentials
		err   error
	)
	creds, err = credentials.NewClientTLSFromFile(certFile, "")
	if err != nil {
		return nil, err
	}
	ma, err := multiaddr.NewMultiaddr(serverAddr)
	if err != nil {
		return nil, err
	}

	netAddr, err := manet.ToNetAddr(ma)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.Dial(netAddr.String(), grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}
	return pb.NewBlockchainServiceClient(conn), nil
}

func makeWalletClient(serverAddr string, rpcCertFile string) (pb.WalletServiceClient, error) {
	certFile := repo.CleanAndExpandPath(rpcCertFile)

	var (
		creds credentials.TransportCredentials
		err   error
	)
	creds, err = credentials.NewClientTLSFromFile(certFile, "")
	if err != nil {
		return nil, err
	}
	ma, err := multiaddr.NewMultiaddr(serverAddr)
	if err != nil {
		return nil, err
	}

	netAddr, err := manet.ToNetAddr(ma)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.Dial(netAddr.String(), grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}
	return pb.NewWalletServiceClient(conn), nil
}
