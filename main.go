package main

import (
	"bytes"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

const (
	chunkSize = 512
)
const (
	undefinedErr        = iota
	fileNotFoundErr     = iota
	accessViolationErr  = iota
	diskFullErr         = iota
	illegalOperationErr = iota
	unknownTIDErr       = iota
	fileExistsErr       = iota
	noSuchUserErr       = iota
)

var (
	ackChans  map[string]chan int
	ackLock   sync.RWMutex
	dataChans map[string]chan []byte
	dataLock  sync.RWMutex
)

func main() {
	ackChans = make(map[string]chan int)
	dataChans = make(map[string]chan []byte)
	pc, err := net.ListenPacket("udp", "0.0.0.0:8069")

	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()
	for {
		buf := make([]byte, 516)
		rn, addr, err := pc.ReadFrom(buf)
		if err != nil {
			log.Fatal(err)
		}
		//log.Println("from:", addr.String(), "network", addr.Network(), "read:", rn, "data:", string(buf))

		switch buf[1] {
		case 1, 2: // read, write
			ack(pc, addr, 0, buf[1]%2)

			endFileName := bytes.IndexByte(buf[2:], 0) + 2
			endMode := bytes.IndexByte(buf[endFileName+1:], 0) + endFileName
			if mode := string(buf[endFileName+1 : endMode+1]); mode != string([]byte("octet")) {
				error(pc, addr, undefinedErr, "only octet mode supported")
				continue
			}

			filename := string(buf[2:endFileName])

			switch buf[1] {
			case 1:
				go func() {
					c := make(chan int, 0)

					ackLock.Lock()
					ackChans[addr.String()] = c
					ackLock.Unlock()

					sendFile(pc, addr, filename, c)

					ackLock.Lock()
					delete(ackChans, addr.String())
					ackLock.Unlock()

					close(c)
				}()
			case 2:
				go func() {
					c := make(chan []byte, 0)

					dataLock.Lock()
					dataChans[addr.String()] = c
					dataLock.Unlock()

					writeFile(pc, addr, filename, c)

					dataLock.Lock()
					delete(dataChans, addr.String())
					dataLock.Unlock()

					close(c)
				}()
			}
		case 3:
			// TODO: use TID here?
			dataLock.RLock()
			if c, ok := dataChans[addr.String()]; ok {
				c <- buf[2:rn]
			}
			dataLock.RUnlock()
		case 4: // received ack
			ackLock.RLock()
			if c, ok := ackChans[addr.String()]; ok {
				c <- int(buf[2])*256 + int(buf[3])
			}
			ackLock.RUnlock()
		case 5: // errors
			endError := bytes.IndexByte(buf[4:], 0) + 4
			log.Println("err_code:", buf[3], "err:", string(buf[4:endError]))
		default:
			error(pc, addr, illegalOperationErr, "illegal operation")
		}
	}
}

func ack(pc net.PacketConn, addr net.Addr, bh, bl byte) {
	_, err := pc.WriteTo([]byte{0, 4, bh, bl, 0}, addr)
	// TODO: try again instead?
	if err != nil {
		log.Fatal(err)
	}
}

func error(pc net.PacketConn, addr net.Addr, code byte, msg string) {
	errPacket := append([]byte{0, 5, 0, code}, []byte(msg)...)
	errPacket = append(errPacket, 0)
	pc.WriteTo(errPacket, addr)
}

func writeFile(pc net.PacketConn, addr net.Addr, filename string, data <-chan []byte) {
	if _, err := os.Stat("files/" + filename); !os.IsNotExist(err) {
		error(pc, addr, fileExistsErr, "file exists")
		return
	}
	file, err := os.Create("files/" + filename)
	if err != nil {
		error(pc, addr, 0, err.Error())
		return
	}
	defer file.Close()

	for {
		buf := <-data
		wn, err := file.Write(buf[2:])
		if err != nil {
			error(pc, addr, 0, err.Error())
			return
		}
		if wn < 512 {
			err := file.Sync()
			if err != nil {
				error(pc, addr, 0, err.Error())
				return
			}
			ack(pc, addr, buf[0], buf[1])
			break
		}
		ack(pc, addr, buf[0], buf[1])
	}
}

func sendFile(pc net.PacketConn, addr net.Addr, filename string, ack <-chan int) {
	if _, err := os.Stat("files/" + filename); os.IsNotExist(err) {
		error(pc, addr, fileNotFoundErr, "file not found")
		return
	}
	file, err := os.Open("files/" + filename)
	if err != nil {
		log.Fatal(err)
	}

	chunk := make([]byte, 0, 516)
	buf := bytes.NewBuffer(chunk)
	for b := 1; ; b++ {
		if b > 65535 {
			b = 0 // large file handling
		}
		// data packet header
		buf.Write([]byte{0, 3, byte(b / 256 % 512), byte(b % 256)})
		wn, err := io.CopyN(buf, file, 512)
		if err != nil && err != io.EOF {
			log.Fatal(err)
		}

		sendBlock := func() {
			_, err := pc.WriteTo(buf.Bytes(), addr)
			if err != nil {
				log.Fatal(err)
			}
		}
		sendBlock()
		for {
			select {
			case ackBlock := <-ack:
				// discard dup ack
				// TODO: test
				if ackBlock < b {
					continue
				}
				if ackBlock != b {
					log.Println("Ack Block MisMatch got:", ackBlock, "expected:", b)
				}
			case <-time.After(time.Second * 5):
				// no ack received
				sendBlock()
				continue
			case <-time.After(time.Second * 24):
				// Just before 5th retry, timeout
				log.Println("get operation timed out")
				return
			}
			break
		}
		// TODO: test filesize % 512 == 0 case
		// works, but write real test
		if wn < 512 {
			break
		}
		buf.Reset()
	}
}
