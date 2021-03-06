package gemini

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/json"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type GeminiAPI struct {
	BaseURL string
	ApiKey string
	ApiSecret string
	Nonce int64
}

// Ticker stores the json returned by the pubticker endpoint
type Ticker struct {
	Bid float64 `json:"bid,string"`
	Ask float64 `json:"ask,string"`
	Last float64 `json:"last,string"`
}

// Fund stores the json returned by the funds endpoint
type Fund struct {
	Type string `json:"type"`
	Currency string `json:"currency"`
	Amount float64 `json:"amount,string"`
	Available float64 `json:"available,string"`
	AvailableForWithdrawal float64 `json:"availableForWithdrawal,string"`
}

// Order stores the json returned by placing an order or getting order status
type Order struct {
	OrderId string `json:"order_id"`
	ClientId string `json:"client_order_id"`
	Symbol string `json:"symbol"`
	Price float64 `json:"price,string"`
	AvgExecPrice float64 `json:"avg_execution_price,string"`
	Side string `json:"side"`
	Type string `json:"type"`
	Timestamp int `json:"timestamp,string"`
	TimestampMs int `json:"timestampms"`
	Live bool `json:"is_live"`
	Cancelled bool `json:"is_cancelled"`
	ExecutedAmount float64 `json:"executed_amount,string"`
	RemainingAmount float64 `json:"remaining_amount,string"`
	OrigAmount float64 `json:"original_amount,string"`
}

// Request is used to set the data for making an api request
type Request interface {
	SetNonce(int64)
	GetPayload() []byte
	GetRoute() string
}

type BaseRequest struct {
	Request string `json:"request"`
	Nonce int64 `json:"nonce"`
}

func (r *BaseRequest) GetPayload() []byte {
	data, _ := json.Marshal(r)
	return data
}

func (r *BaseRequest) GetRoute() string {
	return r.Request
}

func (r *BaseRequest) SetNonce(n int64) {
	r.Nonce = n
}

func NewBaseRequest(route string) BaseRequest {
	return BaseRequest{
		Request: route,
	}
}

type OrderPlaceReq struct {
	BaseRequest
	Symbol string `json:"symbol"`
	Amount string `json:"amount"`
	Price string `json:"price"`
	Side string `json:"side"`
	Type string `json:"type"`
	ClientId string `json:"client_order_id"`
}

func (r *OrderPlaceReq) GetPayload() []byte {
	data, _ := json.Marshal(r)
	return data
}

// AuthAPIReq makes a signed api request to gemini
func (ga *GeminiAPI) AuthAPIReq(r Request) ([]byte, error) {
	client := &http.Client{}
	r.SetNonce(ga.Nonce)
	ga.Nonce++
	req, err := http.NewRequest("POST", fmt.Sprintf("%s%s", ga.BaseURL, r.GetRoute()), nil)
	if err != nil {
		logger.Printf("ERROR: Failed to POST authenticated request to: %s\n", r.GetRoute())
		return []byte{}, nil
	}
	base64Payload := base64.StdEncoding.EncodeToString(r.GetPayload())
	h := hmac.New(sha512.New384, []byte(ga.ApiSecret))
	h.Write([]byte(base64Payload))
	sig := h.Sum(nil)
	req.Header.Add("X-GEMINI-APIKEY", ga.ApiKey)
	req.Header.Add("X-GEMINI-PAYLOAD", base64Payload)
	req.Header.Add("X-GEMINI-SIGNATURE", hex.EncodeToString(sig))
	resp, err := client.Do(req)
	if err != nil {
		logger.Printf("ERROR: failed to POST authenticated request: %s\n", r.GetRoute())
		return []byte{}, nil
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("ERROR: failed to read response body\n")
		return []byte{}, nil
	}
	return body, nil
}

// GetTicker takes a ticker pair and returns a Ticker struct
func (ga *GeminiAPI) GetTicker(pair string) (Ticker, error) {
	tickerUrl := fmt.Sprintf("/v1/pubticker/%s", pair)
	resp, err := http.Get(fmt.Sprintf("%s%s", ga.BaseURL, tickerUrl))
	if err != nil {
		logger.Printf("ERROR: Failed to get ticker for pair %s\n", pair)
		return Ticker{}, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("ERROR: Failed to read ticker from response\n")
		return Ticker{}, err
	}
	ticker := Ticker{}
	err = json.Unmarshal(body, &ticker)
	if err != nil {
		logger.Printf("ERROR: Failed to decode ticker from response\n")
		return ticker, err
	}
	return ticker, nil
}

// GetFunds returns a list of Fund structs
func (ga *GeminiAPI) GetFunds() ([]Fund, error) {
	input := NewBaseRequest("/v1/balances")
	body, err := ga.AuthAPIReq(&input)
	if err != nil {
		logger.Printf("ERROR: Failed to get Funds\n")
		return []Fund{}, err
	}
	funds := []Fund{}
	err = json.Unmarshal(body, &funds)
	if err != nil {
		logger.Printf("ERROR: Failed to get Funds\n")
		return []Fund{}, err
	}
	return funds, nil
}

// GetOrderStatus returns a list of Order structs
func (ga *GeminiAPI) GetOrderStatus() ([]Order, error) {
	input := NewBaseRequest("/v1/orders")
	orders := []Order{}
	body, err := ga.AuthAPIReq(&input)
	if err != nil {
		logger.Printf("ERROR: Failed to get order status\n")
		return []Order{}, err
	}
	err = json.Unmarshal(body, &orders)
	if err != nil {
		logger.Printf("ERROR: Failed to decode order status json\n")
		return []Order{}, err
	}
	return orders, nil
}

// CancelAll attempts to cancel all open orders on the session
func (ga *GeminiAPI) CancelAll() {
	input := NewBaseRequest("/v1/order/cancel/session")
	ga.AuthAPIReq(&input)
}

// PlaceLimitOrder takes a direction, ticker, client_id, amount, and price and returns an Order object
func (ga *GeminiAPI) PlaceLimitOrder(direction, ticker, client_id string, amount, price float64) (Order, error) {
	amountStr := fmt.Sprintf("%0.6f", amount)
	priceStr := ""
	if ticker == "btcusd" || ticker == "ethusd" {
		priceStr = fmt.Sprintf("%0.2f", price)
	} else if ticker == "ethbtc" {
		priceStr = fmt.Sprintf("%0.5f", price)
	} else {
		panic("Unsupported ticker for placing orders")
	}
	body, err := ga.AuthAPIReq(&OrderPlaceReq{
		BaseRequest: NewBaseRequest("/v1/order/new"),
		Symbol: ticker,
		Amount: amountStr,
		Price: priceStr,
		Side: direction,
		Type: "exchange limit",
		ClientId: client_id,
	})
	if err != nil {
		logger.Printf("ERROR: error placing order\n")
		return Order{}, err
	}
	order := Order{}
	err = json.Unmarshal(body, &order)
	if err != nil {
		logger.Printf("ERROR: error decoding order placement json response\n")
		return Order{}, err
	}
	return order, nil
}

// NewGeminiAPI initializes a GeminiAPI object
func NewGeminiAPI(baseurl, apikey, apisecret string) *GeminiAPI {
	ga := &GeminiAPI{
		BaseURL: baseurl,
		ApiKey: apikey,
		ApiSecret: apisecret,
		Nonce: time.Now().UnixNano(),
	}
	logger.Println("Initialized Gemini API")
	return ga
}
