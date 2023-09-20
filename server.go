package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/melbahja/goph"
	"golang.org/x/crypto/ssh"
)

type Server struct {
	Addr string
	Port int `default:"22"`
	User string
	Pass string
	Key  string
	Exec string `default:"ffmpeg"`

	ssh    *goph.Client
	cmd    *goph.Cmd
	cancel context.CancelFunc

	load float64
}

// Dial to the server by ssh protocol
func (s *Server) Dial() (err error) {
	var auth goph.Auth
	if s.Pass != "" {
		auth = goph.Password(s.Pass)
	}
	if s.Key != "" {
		auth, err = goph.Key(s.Key, s.Pass)
		if err != nil {
			return err
		}
	}

	if s.Port == 0 {
		s.Port = 22
	}

	c, err := goph.NewConn(&goph.Config{
		Addr:     s.Addr,
		Port:     uint(s.Port),
		User:     s.User,
		Auth:     auth,
		Timeout:  5 * time.Second,
		Callback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return err
	}

	s.ssh = c
	return nil
}

// CheckLoad read /proc/loadavg and take 5min loading
func (s *Server) CheckLoad() error {
	out, err := s.ssh.Run("cat /proc/loadavg")
	if err != nil {
		return err
	}

	ss := strings.Fields(string(out))

	if len(ss) > 1 {
		s.load, err = strconv.ParseFloat(ss[1], 32)
		if err != nil {
			return err
		}
		return nil
	}

	return errors.New("unexpected state, failed to resolve CPU loading")
}

var ffmpegExecDefault = "ffmpeg"
var ffmpegArgsTmpl = []string{"-hide_banner",
	"-loglevel", "level+error",
	"-timeout", "5000000",
	"-y",
	"-i", "'{{.Addr}}'",
	"-c:v", "mjpeg",
	"-huffman", "optimal",
	"-q:v", "{{.Quality}}",
	"-vf", "fps={{.FrameRate}},realtime",
	"-f", "image2pipe", "-",
}

// StartFFmpeg process on the remote server and copy stdout/stderr
func (s *Server) StartFFmpeg() (err error) {
	var ffmpegArgs []string
	for _, a := range ffmpegArgsTmpl {
		s, err := renderExecTmpl(a, args)
		if err != nil {
			return err
		}
		ffmpegArgs = append(ffmpegArgs, s)
	}

	if s.Exec == "" {
		s.Exec = ffmpegExecDefault
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	s.cmd, err = s.ssh.CommandContext(ctx, s.Exec, ffmpegArgs...)
	if err != nil {
		return err
	}

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to receive stdout pipe: %v", err)
	}

	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to receive stderr pipe: %v", err)
	}

	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)

	return s.cmd.Run()
}

func renderExecTmpl(s string, args interface{}) (string, error) {
	// start ffmpeg
	tmpl, err := template.New("").Parse(s)
	if err != nil {
		return "", err
	}

	buf := bytes.NewBuffer([]byte{})
	if err := tmpl.Execute(buf, args); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}

	if s.cmd != nil {
		s.cmd.Close()
		s.cmd.Session.Close()
	}

	if s.ssh != nil {
		s.ssh.Close()
	}
}
