package btcwallet

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/sirupsen/logrus"
)

type esploraClient struct {
	url string
}

type esploraTx struct {
	Status struct {
		Confirmed bool  `json:"confirmed"`
		BlockTime int64 `json:"block_time"`
	} `json:"status"`
}

func (f *esploraClient) broadcast(txhex string) error {
	endpoint, err := url.JoinPath(f.url, "tx")
	if err != nil {
		return err
	}

	resp, err := http.Post(endpoint, "text/plain", strings.NewReader(txhex))
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var content string // read the response body
		if _, err := resp.Body.Read([]byte(content)); err != nil {
			return fmt.Errorf("broadcast endpoint HTTP error: %s, tx = %s , error = %s", resp.Status, txhex, err.Error())
		}
		return fmt.Errorf("broadcast endpoint HTTP error: %s, tx = %s , error = %s", resp.Status, txhex, content)
	}

	return nil
}

func (f *esploraClient) getTxStatus(txid string) (isConfirmed bool, blocktime int64, err error) {
	endpoint, err := url.JoinPath(f.url, "tx", txid)
	if err != nil {
		return false, 0, err
	}

	resp, err := http.DefaultClient.Get(endpoint)
	if err != nil {
		return false, 0, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, 0, err
	}

	var response esploraTx

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return false, 0, err
	}

	return response.Status.Confirmed, response.Status.BlockTime, nil
}

func (f *esploraClient) getFeeRate() (btcutil.Amount, error) {
	endpoint, err := url.JoinPath(f.url, "fee-estimates")
	if err != nil {
		return 0, err
	}

	resp, err := http.DefaultClient.Get(endpoint)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, errors.New("fee-estimates endpoint HTTP error: " + resp.Status)
	}

	response := make(map[string]float64)

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return 0, err
	}

	if len(response) == 0 {
		logrus.Warn("empty response from esplorea fee-estimates endpoint, default to 2 sat/vbyte")
		return 2.0, nil
	}

	feeRate, ok := response["1"]
	if !ok {
		return 0, errors.New("failed to get fee rate for 1 block")
	}

	return btcutil.Amount(feeRate * 1000), nil
}
