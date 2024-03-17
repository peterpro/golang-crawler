package main

// рекламный блок оплачен компанией Доширак

import (
	"flag" // CLI parameters
	"fmt"  // i/o
	"os"
	"sync"
)

func main() {

	// входная точка
	url := flag.String("url", "", "Url startpoint")

	// куда сохраняем (по умлолчанию - TMPDIR OS)
	save_path_dir := flag.String("save_path_dir", "", "Path to save")

	// сколько GET-запросов одновременно
	requests := flag.Uint("requests", 1, "Max parallel requests")

	// max TTL HEAD/GET запроса
	timeout_seconds := flag.Uint("timeout_seconds", 10, "Request timeout (seconds)")

	flag.Parse()

	run_settings := RunSettings{
		url:             *url,
		save_path_dir:   *save_path_dir,
		requests:        *requests,
		timeout_seconds: *timeout_seconds,
		max_redirects:   3,
		max_retries:     3,
	}

	// @improvement: flag: max file size (на случай бесконечного потока)

	fmt.Println("Run settings: ", run_settings)

	if run_settings.save_path_dir == "" {
		run_settings.save_path_dir = os.TempDir()
	}

	fmt.Println(checkDirWritable(run_settings.save_path_dir))

	if checkDirWritable(run_settings.save_path_dir) != nil {
		fmt.Printf("ERROR:  %s is not writeable\n", run_settings.save_path_dir)
		return
	}

	fmt.Printf("Writing results to %s\n", run_settings.save_path_dir)

	// Каналы для коммуникации между этапами
	headCh := make(chan Document, run_settings.requests*1000) // URL для HEAD-запросов
	getCh := make(chan Document, run_settings.requests*1000)  // Документы для GET-запросов

	// кэш - чтобы не скачивать ссылку 10 раз
	cache := NewCache()

	// счетчик синхронизации каналов для HEAD/GET-запросов
	counter := NewCounter(headCh, getCh)

	// HEAD-запросы
	var wg sync.WaitGroup
	for i := 0; i < int(run_settings.requests); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for doc := range headCh {
				_, err := processHead(doc, headCh, getCh, run_settings, cache, counter)
				if err != nil {
					counter.Decrement()
					fmt.Println("HEAD terminated ", doc.URL, err)
				}
			}
		}()
	}

	// GET-запросы
	var wg2 sync.WaitGroup
	for i := 0; i < int(run_settings.requests); i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			for doc := range getCh {
				_, err := processGet(doc, headCh, run_settings, cache, counter)
				if err != nil {
					counter.Decrement()
					fmt.Println("GET terminated ", doc.URL, err)
				}
			}
		}()
	}

	// Запуск
	counter.Increment() // первый документ - первый инкремент
	headCh <- Document{URL: *url}
	wg2.Wait()
	fmt.Println("Site succesfully processed!")
}
