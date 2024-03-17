package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Структура страницы
type Document struct {
	URL         string
	q_retries   uint
	q_redirects uint
	error       string
}

// тупо упаковать параметры запуска, чтобы не передавать в функции по одному
type RunSettings struct {
	url             string
	save_path_dir   string
	requests        uint
	timeout_seconds uint
	max_retries     uint // для 5xx
	max_redirects   uint // для 3xx
}

// перед тем, как заходить с GET - мы хотим выполнить первоочередные проверки:
// - не проходили ли мы уже эту страницу,
// - тот ли домен,
// - нет ли страницы на диске
// дальше делаем запрос HEAD и смотрим через responseProcessor() очевидные сетевые вещи - 3xx,4xx,5xx
// если всё ОК - отправляем в очередь GET
func processHead(doc Document, headCh chan<- Document, getCh chan<- Document, run_settings RunSettings, cache *Cache, counter *Counter) (Document, error) {
	_, found := cache.Get(doc.URL)
	if found {
		return doc, errors.New("ALREADY_PROCESSED")
	}

	if !checkDomain(&doc, run_settings.url) {
		return doc, errors.New("WRONG_DOMAIN")
	}

	dir_path, filename, err := getPathAndFilenameFromUrl(run_settings.save_path_dir, doc.URL)

	if err != nil {
		return doc, err
	}

	if fileExists(filepath.Join(dir_path, filename)) == nil {
		// dozagruzka
		getCh <- doc
		return doc, nil
	}

	client := &http.Client{
		Timeout: time.Duration(run_settings.timeout_seconds) * time.Second,
	}

	fmt.Println("HEAD: start processing ", doc.URL)
	resp, err := client.Head(doc.URL)
	if err != nil {
		return doc, err
	}
	defer resp.Body.Close()

	_, processError := responseProcessor(resp, headCh, doc, run_settings, counter)
	if processError == nil {
		getCh <- doc
	} else {
		return doc, processError
	}

	return doc, nil
}

// обрабатываем коды, прилетевшие в HEAD/GET
func responseProcessor(resp *http.Response, headCh chan<- Document, doc Document, run_settings RunSettings, counter *Counter) (Document, error) {
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		contentType := resp.Header.Get("Content-Type")
		contentLength := resp.Header.Get("Content-Length")

		if !strings.HasPrefix(contentType, "text/html") &&
			!strings.HasPrefix(contentType, "text/css") &&
			!strings.HasPrefix(contentType, "application/javascript") {
			return doc, errors.New("bad content type")
		}
		if contentLength != "" {
			if length, err := strconv.Atoi(contentLength); err == nil && length <= 0 {
				return doc, errors.New("size <=0")
			}
		}
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		fmt.Println("3xx Redirection, attempt ", doc.q_redirects)
		doc.q_redirects++
		if doc.q_redirects > run_settings.max_redirects {
			doc.error = "MAX_REDIRECTS"
			return doc, errors.New("MAX_REDIRECTS")
		}
		counter.Increment()
		headCh <- doc
		return doc, errors.New("SENT_TO_REDIRECT")
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		fmt.Println("4xx Client Error: ", doc.URL)
		doc.error = fmt.Sprintf("4xx: Got %d code\n", resp.StatusCode)
		return doc, errors.New("4xx")
	case resp.StatusCode >= 500:
		fmt.Println("5xx Server Error, attempt ", doc.q_retries, doc.URL)
		doc.q_retries++
		if doc.q_retries > run_settings.max_retries {
			doc.error = "MAX_RETRIES"
			return doc, errors.New("MAX_RETRIES")
		}
		counter.Increment()
		headCh <- doc
		return doc, errors.New("SENT_TO_RETRY")
	default:
		fmt.Println("Unknown code ", resp.StatusCode, doc.URL)
		doc.error = fmt.Sprintf("Unknown code: Got %d code\n", resp.StatusCode)
		return doc, errors.New("UNKNOWN_CODE")
	}
	return doc, nil
}

func processGet(doc Document, headCh chan<- Document, run_settings RunSettings, cache *Cache, counter *Counter) (Document, error) {

	_, found := cache.Get(doc.URL)
	if found {
		return doc, errors.New("ALREADY_PROCESSED")
	}

	fmt.Println("GET: start processing ", doc.URL)

	dir_path, filename, err := getPathAndFilenameFromUrl(run_settings.save_path_dir, doc.URL)

	if err != nil {
		return doc, err
	}

	// dozagruzka
	if fileExists(filepath.Join(dir_path, filename)) == nil {
		fmt.Println("READING EXISTING FILE ", filepath.Join(dir_path, filename))
		body, err := readFile(filepath.Join(dir_path, filename))
		if err == nil {
			return processPage(doc, headCh, run_settings, cache, counter, body, false)
		}
		return doc, err
	}

	client := &http.Client{
		Timeout: time.Second,
	}

	resp, err := client.Get(doc.URL)
	if err != nil {
		return doc, err
	}
	defer resp.Body.Close()


	_, processError := responseProcessor(resp, headCh, doc, run_settings, counter)
	if processError == nil {

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return doc, err
		}

		return processPage(doc, headCh, run_settings, cache, counter, string(body), true)
	} else {
		return doc, processError
	}
}

func processPage(doc Document, headCh chan<- Document, run_settings RunSettings, cache *Cache, counter *Counter, body string, save bool) (Document, error) {
	cache.Set(doc.URL, "gotcha")
	links := extractLinks(string(body))

	for _, link := range links {
		counter.Increment()
		//fmt.Println("GET: link extracted", doc.URL, link)
		headCh <- Document{URL: link}
	}


	if save {
		dirPath, filename, err := getPathAndFilenameFromUrl(run_settings.save_path_dir, doc.URL)

		if err == nil {
			saveFileFromURL(dirPath, filename, string(body))
		} else {
			return doc, err
		}
	}

	counter.Decrement()
	return doc, nil
}

// cache - не обрабатываем страницы по сотне раз
type Cache struct {
	mu   sync.Mutex
	data map[string]string
}

func NewCache() *Cache {
	return &Cache{
		data: make(map[string]string),
	}
}

func (c *Cache) Set(key string, value string) {
	c.mu.Lock()         //  перед изменением кэша
	defer c.mu.Unlock() //  после завершения
	c.data[key] = value
}

func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()         //  перед чтением кэша
	defer c.mu.Unlock() //  после завершения
	value, ok := c.data[key]
	return value, ok
}

// counter - структура для управления счетчиком урлов обработки с защитой мьютексом
// очень кратко говоря - когда 0, то потребность в каналах закончилась и их можно закрывать
type Counter struct {
	mu     sync.Mutex
	count  int
	closed bool
	ch     chan Document
	ch2    chan Document
}

func NewCounter(channel chan Document, channel2 chan Document) *Counter {
	return &Counter{
		ch:  channel,
		ch2: channel2,
	}
}

func (c *Counter) Increment() {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
}

func (c *Counter) Decrement() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count--

	if c.count == 0 && !c.closed {
		close(c.ch)
		close(c.ch2)
		c.closed = true
	}
}
