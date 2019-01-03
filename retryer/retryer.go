package retryer

import (
	"bytes"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"math"
	"net"
	"net/http"
	"time"
)
const maxRetryCount = 5
const timeout = 30 * time.Second

var DefaultClient = &http.Client{Timeout: timeout}

var retryStatusCodes = map[int]struct{}{
	429: {},
}

type Retryer struct {
	DoFunc func(retryer *Retryer, request *Request) (*http.Response, error)
	client *http.Client
}

func (r *Retryer) Do(request *Request) (*http.Response, error) {
	if r.DoFunc != nil {
		return r.DoFunc(r, request)
	}
	return DoWithExponentialBackoff(r, request)
}

func shouldRetry(statusCode int) bool {
	_, shouldRetry := retryStatusCodes[statusCode]

	if (statusCode >= 500 && statusCode <= 599) || shouldRetry {
		return true
	}
	return false
}

func getWaitTime(retryCount int) time.Duration {
	waitTime := math.Pow(2, float64(retryCount)) * 100
	return time.Duration(waitTime) * time.Millisecond
}

func DoWithExponentialBackoff(retryer *Retryer, request *Request) (*http.Response, error) {

	client := DefaultClient
	if retryer.client != nil {
		client = retryer.client
	}

	retryCount := 0
	for {

		if request.body != nil {
			_, err := request.body.Seek(0, 0)
			if err != nil {
				return nil, err
			}
		}
		response, err := client.Do(request.Request)

		if err, ok := err.(net.Error); ok {
			logrus.Println(err)
			// On error, any Response can be ignored.
			if !err.Timeout() {
				return nil, err
			}
		} else if shouldRetry(response.StatusCode) {
			// If the returned error is nil, the Response will contain a non-nil
			// Body which the user is expected to close.
			io.Copy(ioutil.Discard, response.Body)
			response.Body.Close()
		} else {
			return response, err
		}

		if retryCount < maxRetryCount - 1 {
			waitDuration := getWaitTime(retryCount)
			time.Sleep(waitDuration)
			retryCount++
			continue
		}
		break
	}

	return nil, errors.Errorf("Couldn't get a proper response, maximum retry count[%d] is exceeded.", maxRetryCount)
}

/******************************************************************************************/

type Request struct {
	body io.ReadSeeker
	*http.Request
}

func NewRequest(method, url string, body io.Reader) (*Request, error) {

	rs, ok := body.(io.ReadSeeker)
	if !ok && body != nil {
		data, err := ioutil.ReadAll(body)
		if err != nil {
			return nil, err
		}
		rs = bytes.NewReader(data)
	}

	request, err := http.NewRequest(method, url, rs)
	if err != nil {
		return nil, err
	}
	return &Request{rs, request}, nil
}

