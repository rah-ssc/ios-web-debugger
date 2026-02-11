package webinspector

import (
	"fmt"
	"log"
	"time"
	
	"github.com/danielpaulus/go-ios/ios"
)

type InspectorSession struct {
	UDID      string
	Connected bool
	stopChan  chan bool
}

func ConnectToDevice(udid string) (*InspectorSession, error) {
	log.Printf("Connecting to iOS device: %s", udid)
	
	// Verify device exists
	deviceList, err := ios.ListDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %v", err)
	}
	
	// Check if device is in list
	found := false
	for _, entry := range deviceList.DeviceList {
		if entry.Properties.SerialNumber == udid {
			found = true
			break
		}
	}
	
	if !found {
		return nil, fmt.Errorf("device not found: %s", udid)
	}
	
	// Create session (mock for now)
	session := &InspectorSession{
		UDID:      udid,
		Connected: true,
		stopChan:  make(chan bool),
	}
	
	log.Printf("Connected to device: %s", udid)
	return session, nil
}

func (s *InspectorSession) StartConsoleListening(callback func(string)) {
	go func() {
		callback(fmt.Sprintf("🔌 Connected to iOS device: %s", s.UDID))
		callback("⚠️  Web Inspector not fully implemented yet")
		callback("   Next steps:")
		callback("   1. Enable 'Web Inspector' in iOS Settings")
		callback("   2. Open Safari on iOS device")
		callback("   3. Real console logs will appear here")
		
		// Mock console output for demonstration
		mockLogs := []string{
			"Console cleared",
			"Page loaded: https://example.com",
			"JavaScript executed successfully",
			"Network request: GET /api/data",
			"CSS parsed: style.css",
		}
		
		for i, msg := range mockLogs {
			select {
			case <-s.stopChan:
				return
			case <-time.After(time.Duration(i+1) * time.Second):
				callback(msg)
			}
		}
		
		// Periodic mock logs
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		
		counter := 1
		for {
			select {
			case <-s.stopChan:
				return
			case <-ticker.C:
				callback(fmt.Sprintf("Mock log #%d - Time: %v", counter, time.Now().Format("15:04:05")))
				counter++
			}
		}
	}()
}

func (s *InspectorSession) Close() {
	if s.Connected {
		s.Connected = false
		close(s.stopChan)
		log.Printf("Disconnected from device: %s", s.UDID)
	}
}
