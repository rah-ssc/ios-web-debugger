package proxy

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
)

type IWDPProxy struct {
	cmd       *exec.Cmd
	udid      string
	port      int
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	isRunning bool
	mu        sync.Mutex
	callbacks []func(string)
}

func NewProxy(udid string, port int) *IWDPProxy {
	return &IWDPProxy{
		udid:      udid,
		port:      port,
		callbacks: []func(string){},
	}
}

func (p *IWDPProxy) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.isRunning {
		return fmt.Errorf("proxy already running for device %s", p.udid)
	}
	
	// Start ios-webkit-debug-proxy
	p.cmd = exec.Command("ios_webkit_debug_proxy",
		"-c", fmt.Sprintf("%s:%d", p.udid, p.port),
		"--no-frontend",
	)
	
	var err error
	p.stdout, err = p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}
	
	p.stderr, err = p.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}
	
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start proxy: %v", err)
	}
	
	p.isRunning = true
	
	// Log output
	go p.logOutput(p.stdout, "STDOUT")
	go p.logOutput(p.stderr, "STDERR")
	
	log.Printf("ios-webkit-debug-proxy started for device %s on port %d", p.udid[:8], p.port)
	return nil
}

func (p *IWDPProxy) logOutput(pipe io.ReadCloser, prefix string) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("[%s][%s] %s", p.udid[:8], prefix, line)
		
		// Notify callbacks
		p.mu.Lock()
		for _, cb := range p.callbacks {
			go cb(line)
		}
		p.mu.Unlock()
	}
}

func (p *IWDPProxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.isRunning {
		return nil
	}
	
	if err := p.cmd.Process.Kill(); err != nil {
		log.Printf("Failed to kill proxy for %s: %v", p.udid[:8], err)
	}
	
	p.isRunning = false
	log.Printf("ios-webkit-debug-proxy stopped for device %s", p.udid[:8])
	return nil
}

func (p *IWDPProxy) OnOutput(callback func(string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.callbacks = append(p.callbacks, callback)
}

func (p *IWDPProxy) GetWebSocketURL() string {
	return fmt.Sprintf("ws://localhost:%d/devtools/page/1", p.port)
}
