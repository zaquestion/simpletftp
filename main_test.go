package main

import (
	"github.com/pin/tftp"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

const (
	filePrefix = "testtftp"
)

func TestMain(m *testing.M) {
	// need the server port
	os.Chdir(os.TempDir())
	go main()

	exitCode := m.Run()

	testdirs, err := filepath.Glob(filePrefix + "*")
	if err != nil {
		log.Fatal(err)
	}
	for _, dir := range testfiles {
		err := os.RemoveAll(dir)
		if err != nil {
			log.Println("err:", err)
		}
	}

	os.Exit(exitCode)
}

func TestPut(t *testing.T) {
	d, err := ioutil.TempDir("", filePrefix)
	if err != nil {
		t.Fatal("err:", err)
	}
	os.Chdir(d)

	client, err := tftp.NewClient("put addr here")
	if err != nil {
		t.Fatal("err:", err)
	}

	client.Send(file, "octet")
}
