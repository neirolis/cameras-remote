package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/mattn/go-shellwords"
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

	ssh  *goph.Client
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

// StartFFmpeg process on the remote server and copy stdout/stderr
func (s *Server) StartFFmpeg() error {
	execLine, err := renderExecTmpl(ffmpegExecTmpl, args)
	if err != nil {
		return err
	}

	execArgs, err := shellwords.Parse(execLine)
	if err != nil {
		return err
	}

	cmd, err := s.ssh.Command(s.Exec, execArgs...)
	if err != nil {
		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to receive stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to receive stderr pipe: %v", err)
	}

	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)

	return cmd.Run()
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
