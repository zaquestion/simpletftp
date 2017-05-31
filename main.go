package main

import (
	"bytes"
	"io"
	"log"
	"net"
	"os"
	"time"
)

const (
	chunkSize = 512
)

var (
	ackConn map[string]chan int
)

func main() {
	ackConn = make(map[string]chan int)
	pc, err := net.ListenPacket("udp", "0.0.0.0:8069")

	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()
	for {
		buf := make([]byte, 516)
		_, addr, err := pc.ReadFrom(buf)
		if err != nil {
			log.Fatal(err)
		}
		//log.Println("from:", addr.String(), "read:", rn, "data:", string(buf))

		switch buf[1] {
		case 1: // read
			ack(pc, addr)
			endFileName := bytes.IndexByte(buf[2:], 0) + 2
			endMode := bytes.IndexByte(buf[endFileName+1:], 0) + endFileName
			if mode := string(buf[endFileName+1 : endMode+1]); mode != string([]byte("octet")) {
				//log.Println("got mode:", mode, ", only octet allowed")
				pc.WriteTo(append([]byte{0, 5, 0, 0}, []byte("only octet mode supported")...), addr)
				continue
			}

			filename := string(buf[2:endFileName])

			go func() {
				c := make(chan int, 0)
				ackConn[addr.String()] = c
				sendFile(pc, addr, filename, c)
				close(c)
			}()
		case 4: // received ack
			if c, ok := ackConn[addr.String()]; ok {
				c <- int(buf[2])*256 + int(buf[3])
			}
		case 5:
			// TODO: ack here? (probably)
			ack(pc, addr)
			endError := bytes.IndexByte(buf[4:], 0) + 4
			log.Println("err_code:", buf[3], "err:", string(buf[4:endError]))
		default:
			pc.WriteTo(append([]byte{0, 5, 0, 0}, []byte("Operation Not Supported")...), addr)
		}
	}
}

func ack(pc net.PacketConn, addr net.Addr) {
	_, err = pc.WriteTo([]byte{0, 4, 0, 1}, addr)
	if err != nil {
		log.Fatal(err)
	}
}

func sendFile(pc net.PacketConn, addr net.Addr, filename string, ack <-chan int) {
	if _, err := os.Stat("files/" + filename); os.IsNotExist(err) {
		pc.WriteTo([]byte{0, 5, 0, 1, 0}, addr)
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
		buf.Write([]byte{0, 3, byte(b / 256 % 512), byte(b % 256)})
		copied, err := io.CopyN(buf, file, 512)
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
				// no ack resend
				sendBlock()
			}
			break
		}
		if copied < 512 {
			delete(ackConn, addr.String())
			break
		}
		buf.Reset()
	}
}
