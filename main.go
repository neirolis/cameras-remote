package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jinzhu/configor"
	"github.com/mcuadros/go-defaults"
	"github.com/op/go-logging"
	"github.com/sg3des/argum"
)

var version = "v0.3.4"
var log = logging.MustGetLogger("REMOTE")
var confFilename = "config-remote.yaml"

var args struct {
	Addr      string `argum:"pos,req" help:"rtsp stream address"`
	FrameRate int    `argum:"--framerate" help:"target fps" default:"1"`
	Quality   int    `argum:"--quality" help:"frames quality 1-31" default:"3"`
}

var conf struct {
	Servers []Server `yaml:"servers"`
}

func init() {
	// logFormat := `%{color}[%{module} %{shortfile}] %{message}%{color:reset}`
	logFormat := `[%{module} %{shortfile}] %{message}`
	logging.SetFormatter(logging.MustStringFormatter(logFormat))
	logging.SetBackend(logging.NewLogBackend(os.Stderr, "", 0))

	argum.MustParse(&args)
}

func main() {
	fmt.Fprintln(os.Stderr, "[log] "+version)

	if _, err := os.Stat(confFilename); err != nil {
		log.Fatalf("configuration file '%s' not found", err)
	}
	if err := configor.Load(&conf, confFilename); err != nil {
		log.Fatal(err)
	}

	servers := shuffleServersList()
	if len(servers) == 0 {
		log.Fatal("servers not found")
	}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		servers[0].Stop()
		os.Exit(0)
	}()

	stat, _ := json.Marshal([]Stat{{Name: "server", Text: servers[0].Addr}})
	fmt.Fprintln(os.Stderr, "[stat] "+string(stat))

	if err := servers[0].Dial(); err != nil {
		log.Fatal(err)
	}

	// pick a less loaded server and run ffmpeg
	if err := servers[0].StartFFmpeg(); err != nil {
		log.Fatal(err)
	}
}

type Stat struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

func shuffleServersList() (servers []Server) {
	for _, s := range conf.Servers {
		defaults.SetDefaults(&s)
		servers = append(servers, s)
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(servers), func(i, j int) {
		servers[i], servers[j] = servers[j], servers[i]
	})

	return servers
}
