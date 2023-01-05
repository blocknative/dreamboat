package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/blocknative/dreamboat/pkg/api"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/pkg/errors"
)

const (
	url = "http://localhost:18550" + api.PathGetValidators
)

func main() {
	fmt.Print("getting validators... ")
	if err := getValidators(); err != nil {
		panic(err)
	}
}

func getValidators() error {
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var validators []*types.SignedValidatorRegistration
	if err := json.Unmarshal(body, &validators); err != nil {
		return err
	}

	if len(validators) == 0 {
		return errors.New("empty list of validators")
	}

	fmt.Println(resp.Status)

	return nil
}
