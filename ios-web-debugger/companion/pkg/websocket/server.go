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
	sessions   = make(map[string]*webinspector.InspectorSession) // udid -> session
	sessionsMu sync.Mutex
)

// HandleConnections handles incoming WebSocket connections
func HandleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer ws.Close()
	
	// Register client
	clientsMu.Lock()
	clients[ws] = true
	clientsMu.Unlock()
	
	log.Println("Client connected via WebSocket")
	
	// Send welcome message
	ws.WriteJSON(map[string]interface{}{
		"type": "welcome",
		"data": "iOS Debug Companion Connected",
	})
	
	// Send current device list
	sendDeviceList(ws)
	
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
			handleCommand(ws, cmd)
		}
	}
}

// sendDeviceList sends the current device list to a WebSocket client
func sendDeviceList(ws *websocket.Conn) {
	deviceList, err := ios.ListDevices()
	if err != nil {
		ws.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": "Failed to list devices: " + err.Error(),
		})
		return
	}
	
	// Convert to simple format for web
	var simpleDevices []map[string]interface{}
	for _, entry := range deviceList.DeviceList {
		device := map[string]interface{}{
			"serial":      entry.Properties.SerialNumber,
			"connection":  entry.Properties.ConnectionType,
			"deviceId":    entry.DeviceID,
			"productId":   entry.Properties.ProductID,
			"messageType": entry.MessageType,
		}
		simpleDevices = append(simpleDevices, device)
	}
	
	ws.WriteJSON(map[string]interface{}{
		"type": "devices",
		"data": simpleDevices,
	})
}

// handleCommand processes commands from WebSocket clients
func handleCommand(ws *websocket.Conn, cmd map[string]interface{}) {
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
	case "consoleCommand":
		serial, _ := cmd["serial"].(string)
		command, _ := cmd["consoleCommand"].(string)
		sendConsoleCommand(serial, command)
	default:
		ws.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": fmt.Sprintf("Unknown command: %s", command),
		})
	}
}

// connectToDevice establishes Web Inspector connection to an iOS device
func connectToDevice(ws *websocket.Conn, serial string) {
	ws.WriteJSON(map[string]interface{}{
		"type": "info",
		"data": fmt.Sprintf("Connecting to device: %s...", serial),
	})
	
	// Check if already connected
	sessionsMu.Lock()
	if _, exists := sessions[serial]; exists {
		sessionsMu.Unlock()
		ws.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": "Already connected to this device",
		})
		return
	}
	sessionsMu.Unlock()
	
	// Connect to Web Inspector
	session, err := webinspector.ConnectToDevice(serial)
	if err != nil {
		ws.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": fmt.Sprintf("Connection failed: %v", err),
		})
		log.Printf("Web Inspector connection failed for %s: %v", serial, err)
		return
	}
	
	// Store session
	sessionsMu.Lock()
	sessions[serial] = session
	sessionsMu.Unlock()
	
	// Start listening for console messages
	session.StartConsoleListening(func(msg string) {
		ws.WriteJSON(map[string]interface{}{
			"type":   "console",
			"data":   msg,
			"serial": serial,
		})
	})
	
	// Send success message
	ws.WriteJSON(map[string]interface{}{
		"type":   "connected",
		"data":   fmt.Sprintf("Connected to device: %s", serial),
		"serial": serial,
	})
	
	log.Printf("Web Inspector connected: %s", serial)
}

// disconnectDevice closes Web Inspector connection
func disconnectDevice(serial string) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	
	if session, exists := sessions[serial]; exists {
		session.Close()
		delete(sessions, serial)
		log.Printf("Disconnected from device: %s", serial)
		
		// Notify all clients
		BroadcastToAll("disconnected", map[string]string{
			"serial":  serial,
			"message": "Device disconnected",
		})
	}
}

// sendConsoleCommand sends a JavaScript command to the device console
func sendConsoleCommand(serial string, command string) {
	sessionsMu.Lock()
	session, exists := sessions[serial]
	sessionsMu.Unlock()
	
	if !exists {
		log.Printf("No active session for device: %s", serial)
		return
	}
	
	// Actually use the session variable to avoid "declared and not used" error
	log.Printf("Console command for %s: %s", serial, command)
	_ = session // Use the variable to avoid compiler warning
	// TODO: Implement actual command sending: session.SendCommand(command)
}

// BroadcastToAll sends a message to all connected WebSocket clients
func BroadcastToAll(msgType string, data interface{}) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	
	for client := range clients {
		// Send message without blocking
		go func(c *websocket.Conn) {
			c.WriteJSON(map[string]interface{}{
				"type": msgType,
				"data": data,
			})
		}(client)
	}
}

// GetConnectedSessions returns list of currently connected devices
func GetConnectedSessions() []string {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	
	var connected []string
	for serial := range sessions {
		connected = append(connected, serial)
	}
	return connected
}

// IsDeviceConnected checks if a device is currently connected
func IsDeviceConnected(serial string) bool {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	
	_, exists := sessions[serial]
	return exists
}
