package main

import (
	"math/rand"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/jinzhu/configor"
	"github.com/mcuadros/go-defaults"
	"github.com/op/go-logging"
	"github.com/sg3des/argum"
)

var version = "v0.2.0"
var log = logging.MustGetLogger("REMOTE")
var random = rand.New(rand.NewSource(time.Now().UnixNano()))

var args struct {
	Addr      string `argum:"pos,req" help:"rtsp stream address"`
	FrameRate int    `argum:"--framerate" help:"target fps" default:"5"`
	Quality   int    `argum:"--quality" help:"frames quality 1-31" default:"3"`
}

var conf struct {
	Servers []Server `yaml:"servers"`
}

var ffmpegExecTmpl = "-hide_banner -loglevel level+info -y -i {{.Addr}} -c:v mjpeg -huffman optimal -q:v {{.Quality}} -vf fps={{.FrameRate}},realtime -f image2pipe -"

func init() {
	logFormat := `%{color}[%{module} %{shortfile}] %{message}%{color:reset}`
	logging.SetFormatter(logging.MustStringFormatter(logFormat))
	logging.SetBackend(logging.NewLogBackend(os.Stderr, "", 0))

	argum.MustParse(&args)

	if err := configor.Load(&conf, "config-remote.yaml"); err != nil {
		log.Fatal(err)
	}
}

func main() {
	var servers []*Server
	var wg sync.WaitGroup

	// search for a less loaded server
	for _, s := range conf.Servers {
		defaults.SetDefaults(&s)
		log.Debugf("%+v", s)

		wg.Add(1)
		go func(s *Server) {
			defer wg.Done()

			if err := s.Dial(); err != nil {
				log.Error(s.Addr, err)
				return
			}

			if err := s.CheckLoad(); err != nil {
				log.Error(s.Addr, err)
				return
			}

			servers = append(servers, s)
		}(&s)
	}

	wg.Wait()

	// no servers no work
	if len(servers) == 0 {
		log.Fatal("a suitable server for starting the decoding process has not been found")
	}

	// sort servers by load level
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].load < servers[j].load
	})

	// pick a less loaded server and run ffmpeg
	if err := servers[0].StartFFmpeg(); err != nil {
		log.Fatal(err)
	}
}
