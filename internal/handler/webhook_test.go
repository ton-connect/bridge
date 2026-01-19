package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/ton-connect/bridge/internal/config"
)

func TestSendWebhook(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(2)

	urls := make(chan string, 2)

	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer wg.Done()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		urls <- r.URL.String()
		if string(body) != `{"topic":"test","hash":"test-hash"}` {
			t.Fatalf("bad body: %s", body)
		}
		w.WriteHeader(http.StatusOK)
	})
	hook1 := httptest.NewServer(handlerFunc)
	hook2 := httptest.NewServer(handlerFunc)
	defer hook1.Close()
	defer hook2.Close()

	data := WebhookData{
		Topic: "test",
		Hash:  "test-hash",
	}
	config.Config.WebhookURL = fmt.Sprintf("%s/webhook,%s/callback", hook1.URL, hook2.URL)

	SendWebhook("SOME-CLIENT-ID", data)
	wg.Wait()
	close(urls)

	calledUrls := make(map[string]struct{})
	for url := range urls {
		calledUrls[url] = struct{}{}
	}
	expected := map[string]struct{}{
		"/webhook/SOME-CLIENT-ID":  {},
		"/callback/SOME-CLIENT-ID": {},
	}
	if !reflect.DeepEqual(calledUrls, expected) {
		t.Fatalf("bad urls: %v", calledUrls)
	}
}
