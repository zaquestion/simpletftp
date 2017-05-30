package main

import (
	"log"
	"net"
)

func main() {
	l, err := net.Listen("udp", "0.0.0.0:8069")
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
	for {
		c, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handle(c)
	}
}

func handle(c net.Conn) {
	buf := make([]bytes, 512)
}
