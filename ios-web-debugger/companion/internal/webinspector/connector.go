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
	networkChan chan string
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

// WebKitMessage - Use map instead of struct for flexibility
type WebKitMessage struct {
	ID     int64       `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

func ConnectToDevice(udid string) (*InspectorSession, error) {
	log.Printf("Starting Web Inspector connection to device: %s", udid[:8])
	
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
		networkChan: make(chan string, 1000),
		port:        port,
	}
	
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
			
			for _, page := range pages {
				if page.Title != "ServiceWorker" && page.URL != "about:blank" && page.Title != "" {
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
    log.Printf("🔵 Fetching pages from: %s", url)
    
    resp, err := http.Get(url)
    if err != nil {
        log.Printf("❌ HTTP GET failed: %v", err)
        return nil, err
    }
    defer resp.Body.Close()
    
    log.Printf("✅ HTTP GET successful - Status: %s", resp.Status)
    
    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        log.Printf("❌ Failed to read response body: %v", err)
        return nil, err
    }
    
    log.Printf("📦 Response body length: %d bytes", len(body))
    log.Printf("📦 Raw JSON: %s", string(body))
    
    var pages []Page
    err = json.Unmarshal(body, &pages)
    if err != nil {
        log.Printf("❌ JSON unmarshal failed: %v", err)
        return nil, err
    }
    
    log.Printf("✅ Found %d page(s)", len(pages))
    for i, page := range pages {
        log.Printf("   📄 Page %d:", i)
        log.Printf("       Title: %s", page.Title)
        log.Printf("       URL: %s", page.URL)
        log.Printf("       WebSocketURL: %s", page.WebSocketURL)
        log.Printf("       AppID: %s", page.AppID)
    }
    
    return pages, err
}

func (s *InspectorSession) connectToPage(page Page) error {
    log.Printf("==========================================")
    log.Printf("🔵 CONNECT TO PAGE - START")
    log.Printf("==========================================")
    
    // STEP 1: Check WebSocket URL
    log.Printf("📌 STEP 1: Validating WebSocket URL")
    if page.WebSocketURL == "" {
        log.Printf("❌ ERROR: WebSocketURL is empty")
        return fmt.Errorf("no WebSocket URL")
    }
    
    log.Printf("✅ WebSocketURL exists")
    
    // STEP 2: Log page details
    log.Printf("📌 STEP 2: Page details")
    log.Printf("   Title: '%s'", page.Title)
    log.Printf("   URL: '%s'", page.URL)
    log.Printf("   WebSocketURL: '%s'", page.WebSocketURL)
    log.Printf("   AppID: '%s'", page.AppID)
    
    // STEP 3: DEBUG - Show EXACT URL and properties
    log.Printf("📌 STEP 3: URL Analysis")
    wsURL := page.WebSocketURL
    log.Printf("   🔍 RAW URL: '%s'", wsURL)
    log.Printf("   🔍 URL length: %d characters", len(wsURL))
    log.Printf("   🔍 Starts with 'ws://': %v", strings.HasPrefix(wsURL, "ws://"))
    log.Printf("   🔍 Starts with 'wss://': %v", strings.HasPrefix(wsURL, "wss://"))
    log.Printf("   🔍 Contains '/devtools/page/': %v", strings.Contains(wsURL, "/devtools/page/"))
    
    // STEP 4: Parse URL
    log.Printf("📌 STEP 4: Parsing URL")
    u, err := url.Parse(wsURL)
    if err != nil {
        log.Printf("❌ URL Parse error: %v", err)
        log.Printf("   Error type: %T", err)
        return fmt.Errorf("invalid WebSocket URL: %v", err)
    }
    log.Printf("✅ URL parsed successfully")
    log.Printf("   📍 Scheme: '%s'", u.Scheme)
    log.Printf("   📍 Host: '%s'", u.Host)
    log.Printf("   📍 Path: '%s'", u.Path)
    log.Printf("   📍 RawPath: '%s'", u.RawPath)
    log.Printf("   📍 Fragment: '%s'", u.Fragment)
    log.Printf("   📍 String(): '%s'", u.String())
    
    // STEP 5: Verify it's a page WebSocket, not proxy
    log.Printf("📌 STEP 5: Verifying connection type")
    if strings.Contains(u.Path, "/devtools/page/") {
        log.Printf("✅ Confirmed: This is a PAGE WebSocket (contains /devtools/page/)")
    } else {
        log.Printf("⚠️ WARNING: This does NOT appear to be a page WebSocket!")
        log.Printf("   Expected path to contain '/devtools/page/', got: '%s'", u.Path)
    }
    
    // STEP 6: Attempt connection
    log.Printf("📌 STEP 6: Attempting WebSocket connection")
    log.Printf("   🔌 Dialing: '%s'", u.String())
    
    conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
    if err != nil {
        log.Printf("❌ WebSocket connection FAILED")
        log.Printf("   Error: %v", err)
        log.Printf("   Error type: %T", err)
        
        // Check for common errors
        if strings.Contains(err.Error(), "connection refused") {
            log.Printf("   💡 TIP: Port 9222 may not be listening. Is ios-webkit-debug-proxy running?")
        } else if strings.Contains(err.Error(), "no such host") {
            log.Printf("   💡 TIP: Hostname 'localhost' could not be resolved")
        } else if strings.Contains(err.Error(), "timeout") {
            log.Printf("   💡 TIP: Connection timed out - proxy may be busy")
        }
        
        return fmt.Errorf("WebSocket connection failed: %v", err)
    }
    
    log.Printf("✅ WebSocket connection SUCCESSFUL!")
    log.Printf("   Local address: %s", conn.LocalAddr().String())
    log.Printf("   Remote address: %s", conn.RemoteAddr().String())
    log.Printf("   Subprotocol: %s", conn.Subprotocol())
    
    s.WSConn = conn
    log.Printf("💾 WebSocket connection stored in session")
    
    // STEP 7: Enable Console domain
    log.Printf("📌 STEP 7: Enabling Console domain")
    msgID := int64(1)
    
    consoleMsg := map[string]interface{}{
        "id":     msgID,
        "method": "Console.enable",
    }
    
    log.Printf("   📤 Sending: %v", consoleMsg)
    if err := s.WSConn.WriteJSON(consoleMsg); err != nil {
        log.Printf("❌ Failed to send Console.enable")
        log.Printf("   Error: %v", err)
    } else {
        log.Printf("✅ Console.enable sent successfully (ID: %d)", msgID)
    }
    msgID++
    
    // STEP 8: Enable Runtime domain
    log.Printf("📌 STEP 8: Enabling Runtime domain")
    
    runtimeMsg := map[string]interface{}{
        "id":     msgID,
        "method": "Runtime.enable",
    }
    
    log.Printf("   📤 Sending: %v", runtimeMsg)
    if err := s.WSConn.WriteJSON(runtimeMsg); err != nil {
        log.Printf("❌ Failed to send Runtime.enable")
        log.Printf("   Error: %v", err)
    } else {
        log.Printf("✅ Runtime.enable sent successfully (ID: %d)", msgID)
    }
    msgID++
    
    // STEP 9: Start listener
    log.Printf("📌 STEP 9: Starting message listener")
    go s.listenForMessages()
    log.Printf("✅ Message listener goroutine started")
    
    // STEP 10: Send UI notification
    log.Printf("📌 STEP 10: Sending UI connection message")
    s.consoleChan <- fmt.Sprintf("📱 Connected to: %s", page.Title)
    s.consoleChan <- fmt.Sprintf("🔗 %s", page.URL)
    s.consoleChan <- "──────────────────────────────────────────"
    log.Printf("✅ UI connection messages sent")
    
    log.Printf("==========================================")
    log.Printf("🔵 CONNECT TO PAGE - COMPLETE")
    log.Printf("==========================================")
    
    return nil
}
// ✅ CORRECT: WebKit Protocol Commands for iOS
func (s *InspectorSession) enableWebKitDomains() {
	// msgID := int64(1)
	
	// // 1. Enable Console - WORKS on iOS WebKit
	// s.writeToDevice(WebKitMessage{
	// 	ID:     msgID,
	// 	Method: "Console.enable",
	// })
	// msgID++
	
	// // 2. Enable Runtime - WORKS on iOS WebKit
	// s.writeToDevice(WebKitMessage{
	// 	ID:     msgID,
	// 	Method: "Runtime.enable",
	// })
	// msgID++
	
	// // 3. Enable Page - WORKS on iOS WebKit
	// s.writeToDevice(WebKitMessage{
	// 	ID:     msgID,
	// 	Method: "Page.enable",
	// })
	// msgID++
	
	// // 4. Enable Network - ✅ WORKS on iOS WebKit (different from CDP)
	// s.writeToDevice(WebKitMessage{
	// 	ID:     msgID,
	// 	Method: "Network.enable",
	// })
	// msgID++
	
	// // 5. Optional: Disable cache for network
	// s.writeToDevice(WebKitMessage{
	// 	ID:     msgID,
	// 	Method: "Network.setCacheDisabled",
	// 	Params: map[string]bool{"disabled": true},
	// })
	
	log.Printf("✅ WebKit domains enabled: Console, Runtime, Page, Network")
}

func (s *InspectorSession) writeToDevice(msg interface{}) error {
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
        
        // Parse message
        var envelope struct {
            Method string          `json:"method"`
            Params json.RawMessage `json:"params"`
        }
        if err := json.Unmarshal(msg, &envelope); err != nil {
            continue
        }
        
        // ---------- CONSOLE MESSAGES ----------
        switch envelope.Method {
        case "Runtime.consoleAPICalled":
            var params struct {
                Type string `json:"type"`
                Args []struct {
                    Value interface{} `json:"value"`
                } `json:"args"`
            }
            if err := json.Unmarshal(envelope.Params, &params); err == nil {
                var text string
                for i, arg := range params.Args {
                    if i > 0 {
                        text += " "
                    }
                    text += fmt.Sprintf("%v", arg.Value)
                }
                
                emoji := "📝"
                switch params.Type {
                case "error": emoji = "❌"
                case "warning": emoji = "⚠️"
                case "info": emoji = "ℹ️"
                case "debug": emoji = "🔧"
                }
                
                // 🚨 SEND TO WEB UI - THIS IS MISSING!
                s.consoleChan <- fmt.Sprintf("%s %s", emoji, text)
            }
            
        case "Network.requestWillBeSent":
            var params struct {
                RequestID string `json:"requestId"`
                Request   struct {
                    Method string `json:"method"`
                    URL    string `json:"url"`
                } `json:"request"`
                Type string `json:"type"`
            }
            if err := json.Unmarshal(envelope.Params, &params); err == nil {
                if params.Type == "XHR" || params.Type == "Fetch" {
                    // 🚨 SEND TO WEB UI - THIS IS MISSING!
                    s.consoleChan <- fmt.Sprintf("🌐➡️ %s %s", 
                        params.Request.Method, 
                        params.Request.URL)
                }
            }
            
        case "Network.responseReceived":
            var params struct {
                RequestID string `json:"requestId"`
                Response  struct {
                    URL    string `json:"url"`
                    Status int    `json:"status"`
                } `json:"response"`
            }
            if err := json.Unmarshal(envelope.Params, &params); err == nil {
                emoji := "✅"
                if params.Response.Status >= 400 {
                    emoji = "⚠️"
                }
                if params.Response.Status >= 500 {
                    emoji = "❌"
                }
                // 🚨 SEND TO WEB UI - THIS IS MISSING!
                s.consoleChan <- fmt.Sprintf("🌐⬅️ %s %d %s", 
                    emoji,
                    params.Response.Status, 
                    params.Response.URL)
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
	case "debug":
		emoji = "🔧"
	case "trace":
		emoji = "🔍"
	case "table":
		emoji = "📊"
	case "group", "groupCollapsed":
		emoji = "📁"
	case "groupEnd":
		emoji = "📂"
	case "time", "timeLog":
		emoji = "⏱️"
	case "assert":
		emoji = "❗"
	case "count", "countReset":
		emoji = "🔢"
	case "dir", "dirxml":
		emoji = "📋"
	case "clear":
		emoji = "🧹"
	default:
		emoji = "🔹"
	}
	
	location := ""
	if url != "" {
		parts := strings.Split(url, "/")
		filename := parts[len(parts)-1]
		if filename == "" {
			filename = url
		}
		if len(filename) > 30 {
			filename = filename[:27] + "..."
		}
		location = fmt.Sprintf(" (%s", filename)
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
        callback("📡 Network monitoring enabled")
        callback("──────────────────────")
        
        for {
            select {
            case <-s.stopChan:
                return
            case msg := <-s.consoleChan:
                // 🚨 THIS MUST BE CALLING THE WEBSOCKET WRITE!
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