package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/urfave/cli/v2"
	"go.bobheadxi.dev/zapx/zapx"
	"go.uber.org/zap"
)

var (
	logger *zap.Logger
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
					blockLogger.Info("starting blocknative event watcher")
					<-ctx.Done()
					blockLogger.Info("exiting, goodbye...")
				}()
				go func() {
					defer wg.Done()
					infuraLogger.Info("starting infura logger")
					<-ctx.Done()
					infuraLogger.Info("exiting, goodbye...")
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