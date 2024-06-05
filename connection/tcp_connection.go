package connection

import (
	"bufio"
	"context"
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
	serverConn        net.Conn
	serverHost        string
	serverPort        string
	readChannel       chan byte
	writeChannel      chan byte
	readChannelString chan string
	ctx               context.Context
	ctxCancelFunc     context.CancelFunc
}

// NewTCPConnection creates a new TCP connection to the server provided
func NewTCPConnection(serverHost string, serverPort string) TCPConnection {
	return TCPConnection{
		serverHost:        serverHost,
		serverPort:        serverPort,
		readChannel:       make(chan byte, 64),
		writeChannel:      make(chan byte, 64),
		readChannelString: make(chan string, 8),
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
	return nil
}

// Listen listens to the incoming messages and writes outgoing messages to the connection
func (tcpConn *TCPConnection) Listen() {
	go tcpConn.readFromTCPConnectionAndPostItOnReadChannel()
	go tcpConn.writeToTCPConnectionFromChannel()
}

// Disconnect disconnects form the tcp server and closes all internal channels and cancel all internal contexts
func (tcpConn *TCPConnection) Disconnect() error {
	err := (tcpConn.serverConn).Close()
	if err != nil {
		return err
	}
	close(tcpConn.readChannel)
	close(tcpConn.writeChannel)
	close(tcpConn.readChannelString)
	tcpConn.ctxCancelFunc()
	return nil
}

// ReadStringFromConnection is a blocking call that reads from a channel
func (tcpConn *TCPConnection) ReadStringFromConnection() string {
	return <-tcpConn.readChannelString
}

// Write writes the string data to the TCP connection
func (tcpConn *TCPConnection) Write(data string) {
	dataBytes := []byte(data)
	for _, dataByte := range dataBytes {
		tcpConn.writeChannel <- dataByte
	}
}

// readFromTCPConnectionAndPostItOnReadChannel reads bytes from TCP Connection and posts it on the string channel
func (tcpConn *TCPConnection) readFromTCPConnectionAndPostItOnReadChannel() {
	go func(ctx context.Context) {
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
				if err.Error() == "EOF" {
					errorOccurred = false
					continue
				} else if strings.Contains(err.Error(), "connection reset by peer") {
					// TODO: If connection is reset, then add a retry mechanism for reconnection
					err := tcpConn.Disconnect()
					if err != nil {
						slog.Error("Connection was reset by peers. Error occurred while disconnecting.", "Error", err)
						return
					}
				}
				errorOccurred = true
				continue
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
			case <-ctx.Done():
				slog.Info("Ending readFromTCPConnectionAndPostItOnReadChannel Go routine.")
				return
			default:
				continue
			}
		}
	}(tcpConn.ctx)
}

// writeToTCPConnectionFromChannel writes the data put on the write channel
func (tcpConn *TCPConnection) writeToTCPConnectionFromChannel() {
	go func(ctx context.Context) {
		for byteToBeSent := range tcpConn.writeChannel {
			count, err := (tcpConn.serverConn).Write([]byte{byteToBeSent})
			if err != nil {
				slog.Error("Failed to send byte over TCP.")
				continue
			}
			slog.Debug("Byte sent successfully.", "Byte", byteToBeSent, "Count", count)

			select {
			case <-ctx.Done():
				slog.Info("Ending writeToTCPConnectionFromChannel Go routine.")
				return
			default:
				continue
			}
		}
	}(tcpConn.ctx)
}
