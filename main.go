package main

import (
	"flag" // CLI parameters
	"fmt"  // i/o
	"os"
	"sync"
	"time"
)

// Dummy типы для демонстрации
type Document struct {
	URL         string
	q_retries   uint
	q_redirects uint
	body        string
}

type ParsedDocument struct {
	Document
	Links []string
}

func main() {

	//
	fmt.Println("Hello world!")

	url := flag.String("url", "", "Url startpoint")
	save_path_dir := flag.String("save_path_dir", "", "Path to save")
	requests := flag.Uint("requests", 1, "Max parallel requests")
	timeout_seconds := flag.Uint("timeout_seconds", 10, "Request timeout (seconds)")

	// @improvement: flag: max file size (на случай бесконечного потока)

	flag.Parse()

	fmt.Println("URL: ", *url)
	fmt.Println("parallel requests: ", *requests)
	fmt.Println("request timeout: ", *timeout_seconds, "secs.")

	if *save_path_dir == "" {
		*save_path_dir = os.TempDir()
	}

	fmt.Println(checkDirWritable(*save_path_dir))

	if checkDirWritable(*save_path_dir) != nil {
		fmt.Printf("ERROR:  %s is not writeable\n", *save_path_dir)
		return
	}

	fmt.Printf("Writing results to %s\n", *save_path_dir)

	urls := []string{
		"http://example1.com",
		"http://example2.com",
		"http://example3.com",
		"http://example4.com",
		"http://example5.com",
		"http://example6.com",
		"http://example7.com",
		"http://example8.com",
		"http://example9.com",
		"http://example10.com",
	}

	// рекламный блок оплачен компанией Доширак

	// Каналы для коммуникации между этапами
	headCh := make(chan string, 2)   // URL для HEAD-запросов
	getCh := make(chan Document, 10) // Документы для GET-запросов
	downCh := make(chan Document, 2) // Документы для загрузчика

	// HEAD-запросы
	// todo обработка кодов ошибок
	// todo кэш пройденного
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for url := range headCh {
				fmt.Println("HEAD request to", url)
				fmt.Println(fetchHeadHeaders(url))
				time.Sleep(100 * time.Millisecond) 

				getCh <- Document{URL: url}
			}
		}()
	}

	// GET-запросы
	// todo: paralellize
	// todo обработка кодов ошибок
	var wg2 sync.WaitGroup
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		for doc := range getCh {
			fmt.Println("GET request to", doc.URL, len(headCh), len(getCh), len(downCh))
			time.Sleep(2000 * time.Millisecond)

			downCh <- doc
		}
	}()

	// parser
	// todo: paralellize
	var wg3 sync.WaitGroup
	wg3.Add(1)
	go func() {
		defer wg3.Done()
		for doc := range downCh {
			fmt.Println("Parsing", doc.URL, len(headCh), len(getCh), len(downCh))
		}
	}()

	// todo saver

	// Запуск
	for _, url := range urls {
		headCh <- url
	}

	fmt.Println("Closing headCh")

	// Закрытие канала headCh, чтобы горутина с HEAD-запросами завершилась после обработки всех URL
	close(headCh)

	fmt.Println("wgWait #1")

	wg.Wait() // ждем завершения горутины с headCh

	fmt.Println("Closing getCh")

	// после выполнения headCh обработчика и закрытия headCh, мы закрываем getCh
	close(getCh)

	fmt.Println("wgWait #2")

	wg2.Wait() // Ждем завершения горутины с getCh

	fmt.Println("Crawling finished.")
}
