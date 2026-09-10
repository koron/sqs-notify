package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	valid "github.com/koron/go-valid"
	"github.com/koron/hupwriter"
	"github.com/koron/sqs-notify/v2/internal/cache"
	"github.com/koron/sqs-notify/v2/internal/sqsnotify"
)

const (
	rpSucceed         = "succeed"
	rpIgnoreFailure   = "ignore_failure"
	rpBeforeExecution = "before_execution"
)

func toRP(s string) sqsnotify.RemovePolicy {
	switch s {
	default:
		fallthrough
	case rpSucceed:
		return sqsnotify.Succeed
	case rpIgnoreFailure:
		return sqsnotify.IgnoreFailure
	case rpBeforeExecution:
		return sqsnotify.BeforeExecution
	}
}

func main2() error {
	var (
		cfg     = sqsnotify.NewConfig()
		version bool
		logfile string
		pidfile string

		waitTimeSec  int64
		removePolicy string
		multiplier   int
	)

	flag.StringVar(&cfg.Profile, "profile", "", "AWS profile name")
	flag.StringVar(&cfg.Region, "region", "us-east-1", "AWS region")
	flag.StringVar(&cfg.Endpoint, "endpoint", "", "Endpoint of SQS")
	flag.Var(valid.String(&cfg.QueueName, "").MustSet(), "queue", "SQS queue name")
	flag.BoolVar(&cfg.CreateQueue, "createqueue", false, "create queue if not exists")
	flag.IntVar(&cfg.MaxRetries, "max-retries", cfg.MaxRetries, "max retries for AWS")
	flag.Int64Var(&waitTimeSec, "wait-time-seconds", -1, `wait time in seconds for next polling. (default -1, disabled, use queue default)`)

	flag.StringVar(&cfg.CacheName, "cache", cfg.CacheName,
		`cache name or connection URL
 * memory://?capacity=1000
 * redis://[{USER}:{PASS}@]{HOST}/[{DBNUM}]?[{OPTIONS}]

   DBNUM: redis DB number (default 0)
   OPTIONS:
	* lifetime : lifetime of cachetime (ex. "10s", "2m", "3h")
	* prefix   : prefix of keys

   Example to connect the redis on localhost: "redis://:6379"`)

	flag.IntVar(&cfg.Workers, "workers", cfg.Workers, "num of workers")
	flag.Var(valid.Int(&multiplier, 1).Min(1), "multiplier", `pooling the SQS in multiple runner`)
	flag.DurationVar(&cfg.Timeout, "timeout", 0, "timeout for command execution (default 0 - no timeout)")

	flag.Var(valid.String(&removePolicy, rpSucceed).
		OneOf(rpSucceed, rpIgnoreFailure, rpBeforeExecution), "remove-policy",
		`policy to remove messages from SQS
 * succeed          : after execution, succeeded (default)
 * ignore_failure   : after execution, ignore its result
 * before_execution : before execution`)

	flag.BoolVar(&cfg.AutoExtend, "auto-extend", false, `enables automatic message visibility extension`)
	flag.Var(valid.Float64(&cfg.AutoExtendFactor, 2.0).Min(1.0).Max(10.0),
		"auto-extend-factor", `multiplier for extending the visibility timeout exponentially`)
	flag.Var(valid.Duration(&cfg.AutoExtendMax, 64*time.Minute).Min(time.Minute).Max(4*time.Hour),
		"auto-extend-max", `maximum visibility timeout allowed for a single extension call`)

	flag.BoolVar(&version, "version", false, "show version")
	flag.StringVar(&logfile, "logfile", "", "log file path")
	flag.StringVar(&pidfile, "pidfile", "", "PID file path (require -logfile)")

	if err := valid.Parse(flag.CommandLine, os.Args[1:]); err != nil {
		return err
	}

	if version {
		fmt.Println("sqs-notify2 version:", sqsnotify.Version)
		os.Exit(1)
	}

	if flag.NArg() < 1 {
		return errors.New("need a notification command")
	}
	args := flag.Args()
	cfg.RemovePolicy = toRP(removePolicy)
	cfg.CmdName = args[0]
	cfg.CmdArgs = args[1:]
	if waitTimeSec >= 0 {
		cfg.WaitTime = &waitTimeSec
	}

	if cfg.Workers < 1 {
		return errors.New("\"-worker\" should be greater than 0")
	}
	if cfg.Workers > 10 {
		log.Print("\"WARN: -worker 10+\" doesn't have any effects, check \"-multiplier\"")
	}

	// Setup logger.
	// FIXME: test logging features.
	if pidfile != "" && logfile == "" {
		return errors.New("pidfile option requires logfile option")
	}
	if logfile != "" {
		if logfile == "-" {
			cfg.Logger = log.New(os.Stdout, "", log.LstdFlags)
		} else {
			w, err := hupwriter.New(logfile, pidfile)
			if err != nil {
				return err
			}
			cfg.Logger = log.New(w, "", 0)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	go func() {
		for {
			s := <-sig
			if s == os.Interrupt {
				cancel()
				signal.Stop(sig)
				close(sig)
				return
			}
		}
	}()
	signal.Notify(sig, os.Interrupt)

	c, err := cache.NewCache(ctx, cfg.CacheName)
	if err != nil {
		return err
	}
	defer c.Close()

	var mu sync.Mutex
	var errs []error

	var sg sync.WaitGroup
	sg.Add(multiplier)
	for i := 0; i < multiplier; i++ {
		go func(id int) {
			defer sg.Done()
			err := sqsnotify.New(cfg).Run(ctx, c)
			if isCancel(err) {
				return
			}
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
			log.Printf("inner process #%d is terminated by error: %s", id, err)
		}(i)
	}
	sg.Wait()

	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

func isCancel(err error) bool {
	return errors.Is(err, context.Canceled)
}

func main() {
	err := main2()
	if err != nil {
		log.Fatal(err)
	}
}
