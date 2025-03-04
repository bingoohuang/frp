package frp

import (
	"context"
	"fmt"
	stdlog "log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bingoohuang/ngg/ss"
	"github.com/fatedier/frp/client"
	"github.com/fatedier/frp/pkg/config"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/config/v1/validation"
	"github.com/fatedier/frp/pkg/util/log"
	"github.com/fatedier/frp/pkg/util/util"
	"github.com/fatedier/frp/server"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func Run(cfgFile string) error {
	if cfgFile == "" {
		cfgFile = "~/.frp.yaml"
	}

	var cc struct {
		ServerAddr string `json:"serverAddr,omitempty"`
	}
	if err := yaml.UnmarshalStrict(ss.Pick1(os.ReadFile(util.ExpandFile(cfgFile))), &cc); err != nil {
		stdlog.Printf("W! unmarshal frp config file %s error: %v", cfgFile, err)
	}
	if cc.ServerAddr == "" {
		svrCfg, err := config.LoadServerConfig(cfgFile)
		if err != nil {
			stdlog.Fatalf("load server config error: %v", err)
		}
		svrCfg.Complete()
		warning, err := validation.ValidateServerConfig(svrCfg)
		if warning != nil {
			stdlog.Printf("WARNING: %v\n", warning)
		}
		if err != nil {
			stdlog.Fatalf("validate server config error: %v", err)
		}

		if err := runServer(cfgFile, svrCfg); err != nil {
			stdlog.Fatalf("run server error: %v", err)
		}
		return nil
	}

	// Do not show command usage here.
	if err := runClient(cfgFile); err != nil {
		stdlog.Fatalf("run client error: %v", err)
	}

	return nil
}

func handleTermSignal(svr *client.Service) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	svr.GracefulClose(500 * time.Millisecond)
}

func runClient(cfgFilePath string) error {
	cfg, proxyCfgs, visitorCfgs, err := config.LoadClientConfig(cfgFilePath)
	if err != nil {
		return fmt.Errorf("load client config %s error: %v", cfgFilePath, err)
	}

	warning, err := validation.ValidateAllClientConfig(cfg, proxyCfgs, visitorCfgs)
	if warning != nil {
		stdlog.Printf("WARNING: %v\n", warning)
	}
	if err != nil {
		return err
	}
	return startService(cfg, proxyCfgs, visitorCfgs, cfgFilePath)
}

func startService(
	cfg *v1.ClientCommonConfig,
	proxyCfgs []v1.ProxyConfigurer,
	visitorCfgs []v1.VisitorConfigurer,
	cfgFile string,
) error {
	log.InitLogger(cfg.Log.To, cfg.Log.Level, int(cfg.Log.MaxDays), cfg.Log.DisablePrintColor)

	if cfgFile != "" {
		log.Infof("start frp service for config file [%s]", cfgFile)
		defer log.Infof("frp service for config file [%s] stopped", cfgFile)
	}
	svr, err := client.NewService(client.ServiceOptions{
		Common:         cfg,
		ProxyCfgs:      proxyCfgs,
		VisitorCfgs:    visitorCfgs,
		ConfigFilePath: cfgFile,
	})
	if err != nil {
		return err
	}

	shouldGracefulClose := cfg.Transport.Protocol == "kcp" || cfg.Transport.Protocol == "quic"
	// Capture the exit signal if we use kcp or quic.
	if shouldGracefulClose {
		go handleTermSignal(svr)
	}
	return svr.Run(context.Background())
}

func runServer(cfgFile string, cfg *v1.ServerConfig) (err error) {
	log.InitLogger(cfg.Log.To, cfg.Log.Level, int(cfg.Log.MaxDays), cfg.Log.DisablePrintColor)

	if cfgFile != "" {
		log.Infof("frps uses config file: %s", cfgFile)
	} else {
		log.Infof("frps uses command line arguments for config")
	}

	svr, err := server.NewService(cfg)
	if err != nil {
		return err
	}
	log.Infof("frps started successfully")
	svr.Run(context.Background())
	return
}
