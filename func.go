package main

import (
	"fmt" // i/o
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

func fetchHeadHeaders(url string) (http.Header, error) {
	resp, err := http.Head(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Классификация ответа
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		fmt.Print("2xx Success")
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		fmt.Print("3xx Redirection")
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		fmt.Print("4xx Client Error")
	case resp.StatusCode >= 500:
		fmt.Print("5xx Server Error")
	default:
		fmt.Print("Неизвестный статус ответа")
	}

	// @todo если не text/* - нафиг

	return resp.Header, nil
}

func extractLinks(html string) []string {
	// от знака равно, могут быть одинарные / двойные кавычки, вообще может их не быть (!!111) и упираемся в закрытие тэга в крайнем случае
	re := regexp.MustCompile(`href=["']?([^"'>]+)["']?|src=["']?([^"'>]+)["']?`)

	matches := re.FindAllStringSubmatch(html, -1)

	for _, match := range matches {
		for _, link := range match[1:] {
			if link != "" {
				fmt.Println(link)
			}
		}
	}
	return nil
}

// Директория существует и доступна на запись?
func checkDirWritable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}

	testFile := filepath.Join(path, ".tmp_write_test")
	os.Remove(testFile)
	f, err := os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(testFile)
	return nil
}
