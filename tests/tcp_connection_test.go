package tests

import (
	"log"
	"testing"

	"github.com/therealriteshkudalkar/lis1a2"
	"github.com/therealriteshkudalkar/lis1a2/connection"
)

func TestTCPConnectDisconnect(t *testing.T) {
	var tcpConn = connection.NewTCPConnection("localhost", "4000")
	if err := tcpConn.Connect(); err != nil {
		log.Fatalf("Failed to connect to TCP server.")
	}
	defer func() {
		if err := tcpConn.Disconnect(); err != nil {
			log.Printf("Failed to disconnect from the TCP server.")
		}
	}()

	astmConn := lis1a2.NewASTMConnection(&tcpConn, false)
	err := astmConn.Connect()
	if err != nil {
		return
	}
}
