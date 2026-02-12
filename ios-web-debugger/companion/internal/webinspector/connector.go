package webinspector

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	
	"github.com/gorilla/websocket"
	
	"github.com/yourname/ios-debug-companion/internal/proxy"
)

type InspectorSession struct {
	UDID        string
	Proxy       *proxy.IWDPProxy
	WSConn      *websocket.Conn
	Connected   bool
	Pages       []Page
	stopChan    chan bool
	consoleChan chan string
	port        int
	writeMu     sync.Mutex
}

type Page struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	WebSocketURL string `json:"webSocketDebuggerUrl"`
	AppID        string `json:"appId"`
}

type CDPMessage struct {
	ID     int64       `json:"id,omitempty"`
	Method string      `json:"method,omitempty"`
	Params interface{} `json:"params,omitempty"`
}

func ConnectToDevice(udid string) (*InspectorSession, error) {
	log.Printf("Starting Web Inspector connection to device: %s", udid[:8])
	
	// Use port 9222
	port := 9222
	
	proxy := proxy.NewProxy(udid, port)
	if err := proxy.Start(); err != nil {
		log.Printf("Proxy may already be running: %v", err)
	}
	
	session := &InspectorSession{
		UDID:        udid,
		Proxy:       proxy,
		Connected:   true,
		stopChan:    make(chan bool),
		consoleChan: make(chan string, 1000),
		port:        port,
	}
	
	// Start page discovery
	go session.discoverPages()
	
	return session, nil
}

func (s *InspectorSession) discoverPages() {
	time.Sleep(2 * time.Second)
	
	for s.Connected {
		pages, err := s.getPages()
		if err != nil {
			log.Printf("Failed to get pages: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		
		if len(pages) == 0 {
			time.Sleep(2 * time.Second)
			continue
		}
		
		s.Pages = pages
		
		if s.WSConn == nil && len(pages) > 0 {
			log.Printf("Found %d page(s)", len(pages))
			for i, page := range pages {
				log.Printf("  %d. %s - %s", i+1, page.Title, page.URL)
			}
			
			// Connect to first non-ServiceWorker page
			for _, page := range pages {
				if page.Title != "ServiceWorker" {
					err := s.connectToPage(page)
					if err == nil {
						break
					}
				}
			}
		}
		
		time.Sleep(5 * time.Second)
	}
}

func (s *InspectorSession) getPages() ([]Page, error) {
	url := fmt.Sprintf("http://localhost:%d/json", s.port)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	var pages []Page
	err = json.Unmarshal(body, &pages)
	return pages, err
}

func (s *InspectorSession) connectToPage(page Page) error {
	if page.WebSocketURL == "" {
		return fmt.Errorf("no WebSocket URL")
	}
	
	log.Printf("Connecting to page: %s", page.Title)
	
	wsURL := page.WebSocketURL
	if !strings.HasPrefix(wsURL, "ws://") {
		wsURL = "ws://" + strings.TrimPrefix(wsURL, "http://")
	}
	
	u, err := url.Parse(wsURL)
	if err != nil {
		return fmt.Errorf("invalid WebSocket URL: %v", err)
	}
	
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("WebSocket connection failed: %v", err)
	}
	
	s.WSConn = conn
	log.Printf("✅ Connected to: %s", page.Title)
	
	// Send connection message to UI
	s.consoleChan <- fmt.Sprintf("📱 Connected to: %s", page.Title)
	s.consoleChan <- fmt.Sprintf("🔗 %s", page.URL)
	s.consoleChan <- "──────────────────────────────────────────"
	
	// Enable console domains
	s.enableConsole()
	
	// Start listening for messages
	go s.listenForMessages()
	
	return nil
}

func (s *InspectorSession) enableConsole() {
	enableConsole := CDPMessage{
		ID:     1,
		Method: "Console.enable",
	}
	s.writeToDevice(enableConsole)
	
	enableRuntime := CDPMessage{
		ID:     2,
		Method: "Runtime.enable",
	}
	s.writeToDevice(enableRuntime)
	
	log.Printf("Console domains enabled")
}

func (s *InspectorSession) writeToDevice(msg CDPMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	
	if s.WSConn != nil {
		return s.WSConn.WriteJSON(msg)
	}
	return fmt.Errorf("no device connection")
}

func (s *InspectorSession) listenForMessages() {
	for s.Connected && s.WSConn != nil {
		var msg json.RawMessage
		err := s.WSConn.ReadJSON(&msg)
		if err != nil {
			log.Printf("WebSocket disconnected: %v", err)
			s.WSConn = nil
			break
		}
		
		// Parse message method
		var envelope struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue
		}
		
		// Handle console messages
		switch envelope.Method {
		case "Console.messageAdded":
			var params struct {
				Message struct {
					Level   string `json:"level"`
					Text    string `json:"text"`
					URL     string `json:"url"`
					Line    int    `json:"line"`
				} `json:"message"`
			}
			if err := json.Unmarshal(msg, &params); err == nil {
				s.formatConsoleMessage(params.Message.Level, params.Message.Text, params.Message.URL, params.Message.Line)
			}
			
		case "Runtime.consoleAPICalled":
			var params struct {
				Type  string `json:"type"`
				Args  []struct {
					Type  string      `json:"type"`
					Value interface{} `json:"value"`
				} `json:"args"`
			}
			if err := json.Unmarshal(msg, &params); err == nil {
				var text string
				for i, arg := range params.Args {
					if i > 0 {
						text += " "
					}
					if arg.Value != nil {
						text += fmt.Sprintf("%v", arg.Value)
					}
				}
				s.formatConsoleMessage(params.Type, text, "", 0)
			}
		}
	}
}

func (s *InspectorSession) formatConsoleMessage(level, text, url string, line int) {
	var emoji string
	switch level {
	case "error":
		emoji = "❌"
	case "warning", "warn":
		emoji = "⚠️"
	case "info":
		emoji = "ℹ️"
	case "log":
		emoji = "📝"
	default:
		emoji = "🔹"
	}
	
	location := ""
	if url != "" {
		shortURL := url
		if len(url) > 50 {
			shortURL = url[:47] + "..."
		}
		location = fmt.Sprintf(" (%s", shortURL)
		if line > 0 {
			location += fmt.Sprintf(":%d", line)
		}
		location += ")"
	}
	
	formatted := fmt.Sprintf("%s %s%s", emoji, text, location)
	
	select {
	case s.consoleChan <- formatted:
	default:
	}
}

func (s *InspectorSession) StartConsoleListening(callback func(string)) {
	go func() {
		callback("🔌 Web Inspector Active")
		callback("──────────────────────")
		
		for {
			select {
			case <-s.stopChan:
				return
			case msg := <-s.consoleChan:
				callback(msg)
			}
		}
	}()
}

func (s *InspectorSession) Close() {
	if s.Connected {
		s.Connected = false
		close(s.stopChan)
		
		if s.WSConn != nil {
			s.WSConn.Close()
		}
		
		if s.Proxy != nil {
			s.Proxy.Stop()
		}
		
		log.Printf("Disconnected from device: %s", s.UDID[:8])
	}
}
