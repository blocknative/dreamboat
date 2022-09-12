package main

import (
	"fmt"
	"io"
	"net/http"

	relay "github.com/blocknative/dreamboat/pkg"
	"github.com/pkg/errors"
)

const (
	url = "http://localhost:18550" + relay.PathStatus
)

func main() {
	fmt.Print("checking status... ")
	if err := status(); err != nil {
		panic(err)
	}
}

func status() error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return errors.WithMessage(fmt.Errorf("invalid return code, expected 200 - received %d", resp.StatusCode), string(body))
	}

	fmt.Println(resp.Status)
	return nil
}
