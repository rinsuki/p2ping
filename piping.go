package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"errors"

	"github.com/google/uuid"
)

type PipingServerPipe struct {
	Reader *bufio.Reader
	Writer io.Writer
}

func (p *PipingServerPipe) WriteMessage(typ string, body string) {
	p.Writer.Write([]byte(fmt.Sprintf("%s|%s\n", typ, body)))
}

func writeToStderr(f string) {
	if _, err := os.Stderr.WriteString(f); err != nil {
		panic(err)
	}
}

func NewPipeAsServer(serverOrURL string) PipingServerPipe {
	var url string
	if strings.HasPrefix(serverOrURL, "https://") || strings.HasPrefix(serverOrURL, "http://") {
		// it seems url
		url = serverOrURL
	} else {
		u, err := uuid.NewRandom()
		if err != nil {
			panic(err)
		}
		uu := u.String()

		url = fmt.Sprintf("https://%s/%s", serverOrURL, uu)
	}
	writeToStderr(fmt.Sprintf("[p2ping] Please Run p2ping %s\n", url))

	readConnRes, err := http.Get(url)
	if readConnRes.StatusCode != 200 {
		panic(errors.New(fmt.Sprintf("readConnRes.StatusCode != 200 (%d)", readConnRes.StatusCode)))
	}
	writeToStderr("[p2ping] piping: client->server connected\n")
	reader := bufio.NewReaderSize(readConnRes.Body, 1024*1024)
	lineBytes, isPrefix, err := reader.ReadLine()
	if err != nil || isPrefix {
		panic(err)
	}
	writeURL := string(lineBytes)

	writeReader, writer := io.Pipe()
	go http.Post(writeURL, "application/octet-stream", writeReader)
	writer.Write([]byte("connected\n"))
	writeToStderr("[p2ping] piping: server->client connected\n")

	return PipingServerPipe{
		Reader: reader,
		Writer: writer,
	}
}

func NewPipeAsClient(sendURL string) PipingServerPipe {
	u, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}
	uu := u.String()

	sendURLParsed, err := url.Parse(sendURL)

	url := fmt.Sprintf("https://%s/%s", sendURLParsed.Host, uu)
	writeReader, writer := io.Pipe()
	go http.Post(sendURL, "application/octet-stream", writeReader)
	writeToStderr("[p2ping] piping: client->server connected\n")
	writer.Write([]byte(url + "\n"))

	readConnRes, err := http.Get(url)
	if readConnRes.StatusCode != 200 {
		panic(errors.New(fmt.Sprintf("readConnRes.StatusCode != 200 (%d)", readConnRes.StatusCode)))
	}
	writeToStderr("[p2ping] piping: server->client connected\n")
	reader := bufio.NewReaderSize(readConnRes.Body, 1024*1024)
	lineBytes, isPrefix, err := reader.ReadLine()
	if err != nil || isPrefix {
		panic(err)
	}
	if string(lineBytes) != "connected" {
		panic(fmt.Errorf("Unknown Hello Message"))
	}

	return PipingServerPipe{
		Reader: reader,
		Writer: writer,
	}
}
