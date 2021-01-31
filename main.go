package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	bindings "github.com/bonedaddy/blocknative-infura-event-test/bindings/pool"
	"github.com/bonedaddy/go-blocknative/client"
	blockclient "github.com/bonedaddy/go-blocknative/client"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gorilla/websocket"
	"github.com/urfave/cli/v2"
	"go.bobheadxi.dev/zapx/zapx"
	"go.uber.org/zap"
)

var (
	logger     *zap.Logger
	logSwapABI = `{
		"anonymous": false,
		"inputs": [
		  {
			"indexed": true,
			"internalType": "address",
			"name": "caller",
			"type": "address"
		  },
		  {
			"indexed": true,
			"internalType": "address",
			"name": "tokenIn",
			"type": "address"
		  },
		  {
			"indexed": true,
			"internalType": "address",
			"name": "tokenOut",
			"type": "address"
		  },
		  {
			"indexed": false,
			"internalType": "uint256",
			"name": "tokenAmountIn",
			"type": "uint256"
		  },
		  {
			"indexed": false,
			"internalType": "uint256",
			"name": "tokenAmountOut",
			"type": "uint256"
		  }
		],
		"name": "LOG_SWAP",
		"type": "event"
	}`
)

func main() {
	app := cli.NewApp()
	app.Name = "blocknative-infura-event-test"
	app.Usage = "compare blocknative and infura to see which one picks up events faster"
	app.Before = func(c *cli.Context) (err error) {
		logger, err = zapx.New(c.String("log.path"), c.Bool("log.dev"))
		return
	}
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "log.dev",
			Usage: "enable dev logging",
			Value: true,
		},
		&cli.StringFlag{
			Name:  "log.path",
			Value: "infura_blocknative.log",
			Usage: "file to store logs",
		},
		&cli.StringFlag{
			Name:    "infura.api_key",
			Usage:   "api key for acessing infrua",
			EnvVars: []string{"INFURA_API_KEY"},
		},
		&cli.StringFlag{
			Name:    "blocknative.api_key",
			Usage:   "api key for accessing blocknative",
			EnvVars: []string{"BLOCKNATIVE_API"},
		},
		&cli.StringFlag{
			Name:  "blocknative.scheme",
			Usage: "connection scheme to use",
			Value: "wss",
		},
		&cli.StringFlag{
			Name:  "blocknative.host",
			Usage: "host to connect to",
			Value: "api.blocknative.com",
		},
		&cli.StringFlag{
			Name:  "blocknative.api_path",
			Usage: "api path to use",
			Value: "/v0",
		},
		&cli.StringFlag{
			Name:  "defi5.address",
			Usage: "defi5 smart contract address",
			Value: "0xfa6de2697D59E88Ed7Fc4dFE5A33daC43565ea41",
		},
	}
	app.Commands = cli.Commands{
		&cli.Command{
			Name:  "start",
			Usage: "start event comparison process",
			Action: func(c *cli.Context) error {
				ctx, cancel := context.WithCancel(c.Context)
				ch := make(chan os.Signal, 1)
				signal.Notify(ch, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, os.Interrupt, os.Kill)
				blockLogger := logger.Named("blocknative")
				infuraLogger := logger.Named("infura")
				wg := &sync.WaitGroup{}
				wg.Add(2)
				go func() {
					defer wg.Done()
					blocknativeListen(ctx, c, blockLogger)
				}()
				go func() {
					defer wg.Done()
					infuraListen(ctx, c, infuraLogger)
				}()
				<-ch
				cancel()
				wg.Wait()
				return nil
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		logger.Fatal("failed to run app", zap.Error(err))
	}

}

func infuraListen(ctx context.Context, c *cli.Context, infuraLogger *zap.Logger) {
	client, err := ethclient.Dial("wss://mainnet.infura.io/ws/v3/" + c.String("infura.api_key"))
	if err != nil {
		infuraLogger.Error("failed to get infura client", zap.Error(err))
		return
	}
	contract, err := bindings.NewPoolbindings(common.HexToAddress(c.String("defi5.address")), client)
	if err != nil {
		infuraLogger.Error("failed to get contract bindings", zap.Error(err))
		return
	}
	ch := make(chan *bindings.PoolbindingsLOGSWAP)
	sub, err := contract.WatchLOGSWAP(nil, ch, nil, nil, nil)
	if err != nil {
		infuraLogger.Error("failed to get LOGSWAP subscription", zap.Error(err))
		return
	}
	infuraLogger.Info("starting infura event watcher")
	for {
		select {
		case err := <-sub.Err():
			infuraLogger.Error("error parsing event", zap.Error(err))
			continue
		case evLog := <-ch:
			infuraLogger.Info("found new event", zap.Any("log", evLog))
		case <-ctx.Done():
			infuraLogger.Info("exiting, goodbye...")
			return
		}
	}
}

func blocknativeListen(ctx context.Context, c *cli.Context, blockLogger *zap.Logger) {
	bclient, err := blockclient.New(ctx, blockclient.Opts{
		Scheme: c.String("blocknative.scheme"),
		Host:   c.String("blocknative.host"),
		Path:   c.String("blocknative.api_path"),
		APIKey: c.String("blocknative.api_key"),
	})
	if err != nil {
		blockLogger.Error("failed to get blocknative client", zap.Error(err))
		return
	}
	if err := bclient.WriteJSON(
		client.NewConfiguration(
			client.NewBaseMessageMainnet(bclient.APIKey()),
			client.NewConfig(
				"global",
				false,
				[]string{logSwapABI},
				nil,
			),
		),
	); err != nil {
		blockLogger.Error("failed to write blocknative client config", zap.Error(err))
		return
	}
	// drain
	var out interface{}
	_ = bclient.ReadJSON(&out)
	if err := bclient.WriteJSON(client.NewAddressSubscribe(
		client.NewBaseMessageMainnet(
			bclient.APIKey(),
		),
		c.String("defi5.address"),
	)); err != nil {
		blockLogger.Error("failed to subscribe to defi5 contract", zap.Error(err))
		return
	}
	// drain
	_ = bclient.ReadJSON(nil)
	blockLogger.Info("starting blocknative event watcher")
	for {
		select {
		case <-ctx.Done():
			blockLogger.Info("exiting, goodbye...")
			return
		default:
		}
		var out client.EthTxPayload
		if err := bclient.ReadJSON(&out); err != nil {
			// used to ignore the following event
			// websocket: close 1005 (no status)
			if websocket.IsUnexpectedCloseError(err) {
				blockLogger.Warn("received unexpected closed running reinit", zap.Error(err))
				if err = bclient.ReInit(); err != nil {
					blockLogger.Error("failed to recover reinit exiting", zap.Error(err))
					return
				}
				blockLogger.Warn("recovered with reinit continuing")
				// recovered successfully
				continue
			}
		}
		blockLogger.Info("received event message", zap.Any("message", out))
	}
}
