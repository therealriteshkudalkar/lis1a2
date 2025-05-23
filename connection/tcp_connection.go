package connection

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/therealriteshkudalkar/lis1a2/constants"
)

// NOTE: It's okay to copy the context object and the net.Conn object,
// because their underlying data is passed by reference

type TCPConnection struct {
	isConnected       bool
	serverConn        net.Conn
	serverHost        string
	serverPort        string
	writeChannel      chan byte
	readChannelString chan string
	ctx               context.Context
	ctxCancelFunc     context.CancelFunc
}

// NewTCPConnection creates a new TCP connection to the server provided
func NewTCPConnection(serverHost string, serverPort string) TCPConnection {
	return TCPConnection{
		isConnected: false,
		serverHost:  serverHost,
		serverPort:  serverPort,
	}
}

// Connect connects to the tcp server
func (tcpConn *TCPConnection) Connect() error {
	serverAddress := fmt.Sprintf("%v:%v", tcpConn.serverHost, tcpConn.serverPort)
	conn, err := net.Dial("tcp", serverAddress)
	if err != nil {
		return err
	}
	tcpConn.serverConn = conn
	tcpConn.ctx, tcpConn.ctxCancelFunc = context.WithCancel(context.Background())
	tcpConn.isConnected = true
	tcpConn.writeChannel = make(chan byte, 64)
	tcpConn.readChannelString = make(chan string, 8)
	return nil
}

// IsConnected gives connection status
func (tcpConn *TCPConnection) IsConnected() bool {
	return tcpConn.isConnected
}

// Listen listens to the incoming messages and writes outgoing messages to the connection
func (tcpConn *TCPConnection) Listen() {
	go tcpConn.readFromTCPConnectionAndPostItOnReadChannel()
	go tcpConn.writeToTCPConnectionFromChannel()
}

// Disconnect disconnects form the tcp server and closes all internal channels and cancel all internal contexts
func (tcpConn *TCPConnection) Disconnect() error {
	tcpConn.ctxCancelFunc()
	close(tcpConn.writeChannel)
	close(tcpConn.readChannelString)
	tcpConn.isConnected = false
	if err := (tcpConn.serverConn).Close(); err != nil {
		return err
	}
	return nil
}

// ReadStringFromConnection is a blocking call that reads from a channel
func (tcpConn *TCPConnection) ReadStringFromConnection() (string, error) {
	str, ok := <-tcpConn.readChannelString
	if !ok {
		return "", errors.New("reading from a closed channel")
	}
	return str, nil
}

// Write writes the bytes array to the TCP connection
func (tcpConn *TCPConnection) Write(data []byte) {
	for _, dataByte := range data {
		tcpConn.writeChannel <- dataByte
	}
}

// readFromTCPConnectionAndPostItOnReadChannel reads bytes from TCP Connection and posts it on the string channel
func (tcpConn *TCPConnection) readFromTCPConnectionAndPostItOnReadChannel() {
	var buffer = make([]byte, 0)
	var errorOccurred = false
	var reader = bufio.NewReader(tcpConn.serverConn)
	for {
		if errorOccurred {
			errorOccurred = false
			time.Sleep(time.Second * 1)
		}
		bt, err := reader.ReadByte()
		if err != nil {
			errorMessage := err.Error()
			if strings.Contains(errorMessage, "EOF") {
				if err := tcpConn.Disconnect(); err != nil {
					slog.Error("End of file encountered! Error occurred while disconnecting.", "Error", err)
					return
				}
				slog.Info("End of file encountered! Disconnected successfully.")
				return
			} else if strings.Contains(errorMessage, "connection reset by peer") {
				if err := tcpConn.Disconnect(); err != nil {
					slog.Error("Connection was reset by peers. Error occurred while disconnecting.", "Error", err)
					return
				}
				slog.Info("Connection was reset by peers. Disconnected successfully.")
				return
			} else if strings.Contains(errorMessage, "use of closed network connection") {
				if err := tcpConn.Disconnect(); err != nil {
					slog.Error("Stopped using closed network connection. Error occurred while disconnecting. ", "Error", err)
					return
				}
				slog.Info("Stopped using closed network connection. Disconnected successfully.")
				return
			} else {
				slog.Error("Some error occurred while reading a byte.", "Error", err)
				errorOccurred = true
				continue
			}
		}

		if bt == constants.NUL {
			continue
		}
		if bt == constants.ENQ || bt == constants.ACK || bt == constants.NAK || bt == constants.EOT {
			buffer = make([]byte, 0)
			buffer = append(buffer, bt)
			tcpConn.readChannelString <- string(buffer)
			buffer = make([]byte, 0)
		} else if bt == constants.STX {
			// start of frame
			buffer = make([]byte, 0)
			buffer = append(buffer, bt)
		} else if bt == constants.LF {
			buffer = append(buffer, bt)
			tcpConn.readChannelString <- string(buffer)
		} else {
			buffer = append(buffer, bt)
		}

		select {
		case <-tcpConn.ctx.Done():
			slog.Info("Ending readFromTCPConnectionAndPostItOnReadChannel Go routine.")
			return
		default:
			continue
		}
	}
}

// writeToTCPConnectionFromChannel writes the data put on the write channel
func (tcpConn *TCPConnection) writeToTCPConnectionFromChannel() {
	for byteToBeSent := range tcpConn.writeChannel {
		count, err := (tcpConn.serverConn).Write([]byte{byteToBeSent})
		if err != nil {
			slog.Error("Failed to send byte over TCP.")
			continue
		}
		slog.Debug("Byte sent successfully.", "Byte", byteToBeSent, "Count", count)
	}
	slog.Info("Ending writeToTCPConnectionFromChannel Go routine.")
}
