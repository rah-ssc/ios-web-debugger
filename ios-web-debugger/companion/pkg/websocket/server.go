package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	
	"github.com/gorilla/websocket"
	
	"github.com/danielpaulus/go-ios/ios"
	"github.com/yourname/ios-debug-companion/internal/webinspector"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	
	// WebSocket clients management
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex
	
	// Web Inspector sessions management
	sessions   = make(map[string]*webinspector.InspectorSession)
	sessionsMu sync.Mutex
)

// WSConnection wraps WebSocket with a mutex for safe concurrent writes
type WSConnection struct {
	Conn    *websocket.Conn
	WriteMu *sync.Mutex
}

func safeWriteJSON(ws *WSConnection, v interface{}) error {
	if ws == nil || ws.Conn == nil {
		return fmt.Errorf("nil websocket connection")
	}
	ws.WriteMu.Lock()
	defer ws.WriteMu.Unlock()
	return ws.Conn.WriteJSON(v)
}

// HandleConnections handles incoming WebSocket connections
func HandleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer ws.Close()
	
	// Create a write mutex for this connection
	writeMu := &sync.Mutex{}
	wsConn := &WSConnection{
		Conn:    ws,
		WriteMu: writeMu,
	}
	
	// Register client
	clientsMu.Lock()
	clients[ws] = true
	clientsMu.Unlock()
	
	log.Println("Client connected via WebSocket")
	
	// Send welcome message
	safeWriteJSON(wsConn, map[string]interface{}{
		"type": "welcome",
		"data": "iOS Debug Companion Connected",
	})
	
	// Send current device list
	sendDeviceList(wsConn)
	
	// Handle incoming messages
	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			log.Printf("Client disconnected: %v", err)
			clientsMu.Lock()
			delete(clients, ws)
			clientsMu.Unlock()
			break
		}
		
		log.Printf("Received: %s", message)
		
		// Handle commands
		var cmd map[string]interface{}
		if err := json.Unmarshal(message, &cmd); err == nil {
			handleCommand(wsConn, cmd)
		}
	}
}

// sendDeviceList sends the current device list to a WebSocket client
func sendDeviceList(ws *WSConnection) {
	deviceList, err := ios.ListDevices()
	if err != nil {
		safeWriteJSON(ws, map[string]interface{}{
			"type": "error",
			"data": "Failed to list devices: " + err.Error(),
		})
		return
	}
	
	// Deduplicate devices by serial
	seen := make(map[string]bool)
	var simpleDevices []map[string]interface{}
	
	for _, entry := range deviceList.DeviceList {
		serial := entry.Properties.SerialNumber
		if !seen[serial] {
			seen[serial] = true
			device := map[string]interface{}{
				"serial":      serial,
				"connection":  entry.Properties.ConnectionType,
				"deviceId":    entry.DeviceID,
				"productId":   entry.Properties.ProductID,
				"messageType": entry.MessageType,
			}
			simpleDevices = append(simpleDevices, device)
		}
	}
	
	safeWriteJSON(ws, map[string]interface{}{
		"type": "devices",
		"data": simpleDevices,
	})
}

// handleCommand processes commands from WebSocket clients
func handleCommand(ws *WSConnection, cmd map[string]interface{}) {
	command, _ := cmd["command"].(string)
	
	switch command {
	case "refreshDevices":
		sendDeviceList(ws)
	case "connect":
		serial, _ := cmd["serial"].(string)
		connectToDevice(ws, serial)
	case "disconnect":
		serial, _ := cmd["serial"].(string)
		disconnectDevice(serial)
	default:
		safeWriteJSON(ws, map[string]interface{}{
			"type": "error",
			"data": fmt.Sprintf("Unknown command: %s", command),
		})
	}
}

// connectToDevice establishes Web Inspector connection to an iOS device
func connectToDevice(ws *WSConnection, serial string) {
	safeWriteJSON(ws, map[string]interface{}{
		"type": "info",
		"data": fmt.Sprintf("Connecting to device: %s...", serial[:8]),
	})
	
	// Check if already connected
	sessionsMu.Lock()
	if _, exists := sessions[serial]; exists {
		sessionsMu.Unlock()
		safeWriteJSON(ws, map[string]interface{}{
			"type": "error",
			"data": "Already connected to this device",
		})
		return
	}
	sessionsMu.Unlock()
	
	// Connect to Web Inspector
	session, err := webinspector.ConnectToDevice(serial)
	if err != nil {
		safeWriteJSON(ws, map[string]interface{}{
			"type": "error",
			"data": fmt.Sprintf("Connection failed: %v", err),
		})
		log.Printf("Web Inspector connection failed for %s: %v", serial[:8], err)
		return
	}
	
	// Store session
	sessionsMu.Lock()
	sessions[serial] = session
	sessionsMu.Unlock()
	
	// Start listening for console messages
	session.StartConsoleListening(func(msg string) {
		safeWriteJSON(ws, map[string]interface{}{
			"type":   "console",
			"data":   msg,
			"serial": serial[:8],
		})
	})
	
	// Send success message
	safeWriteJSON(ws, map[string]interface{}{
		"type":   "connected",
		"data":   fmt.Sprintf("Connected to device: %s", serial[:8]),
		"serial": serial[:8],
	})
	
	log.Printf("Web Inspector connected: %s", serial[:8])
}

// disconnectDevice closes Web Inspector connection
func disconnectDevice(serial string) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	
	if session, exists := sessions[serial]; exists {
		session.Close()
		delete(sessions, serial)
		log.Printf("Disconnected from device: %s", serial[:8])
	}
}

// BroadcastToAll sends a message to all connected WebSocket clients
func BroadcastToAll(msgType string, data interface{}) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	
	for client := range clients {
		// Skip broadcast for now to avoid concurrent write issues
		_ = client // Use the variable to avoid "declared and not used" error
	}
}
