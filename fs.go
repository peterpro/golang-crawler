package main

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

/**
Сборник функций для работы с файловой системой, именами файлов, ссылками и т.д.
*/

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

func fileExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return err
		}
		return err
	}

	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}

	return err
}

func readFile(path string) (string, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// т.к. урл может содержать ?param=hello+world&param2
func escapeFilename(filename string) string {
	replacer := strings.NewReplacer("?", "_", "&", "_", "+", "_")
	return replacer.Replace(filename)
}

// приводим путь страницы к пути сохранения
func getPathAndFilenameFromUrl(base_path string, rawURL string) (string, string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}

	host := parsedURL.Hostname()
	path := parsedURL.Path
	query := parsedURL.RawQuery

	filename := escapeFilename(path)
	if query != "" {
		filename += escapeFilename(query)
	}

	if filename == "" {
		filename = "generic_index.html" // если это корень директории /
	}

	// Собираем полный путь к директории
	dirPath := filepath.Join(base_path, host, filepath.Dir(path))

	return dirPath, filepath.Base(filename), nil

}

// метод жёстко выдирает ссылки из кода, ходит только по href и src
//
// @improvement стоило бы использовать нормальную библиотеку с парсингом DOM-дерева, но это решение намного более парето-оптимальное по времени
func extractLinks(html string) []string {
	// от знака равно, могут быть одинарные / двойные кавычки, вообще может их не быть (!!111) и упираемся в закрытие тэга в крайнем случае
	re := regexp.MustCompile(`href=["']?([^"'>]+)["']?|src=["']?([^"'>]+)["']?`)

	matches := re.FindAllStringSubmatch(html, -1)

	var links []string

	// слайс с совпадениями, каждое совпадение - это слайс [хреф=ссылка, ссылка]
	for _, match := range matches {
		for _, link := range match[1:] {
			if link != "" {
				links = append(links, link)
			}
		}
	}
	return links
}

// метод записывает в файл содержимое content
func saveFileFromURL(dir_path string, filename string, content string) error {

	filePath := filepath.Join(dir_path, filepath.Base(filename))

	if err := os.MkdirAll(dir_path, os.ModePerm); err != nil {
		fmt.Println("DIRECTORY CREATION ERROR ", err)
		return err
	}

	file, err := os.Create(filePath)
	if err != nil {
		fmt.Println("FILE", filePath, " CREATION ERROR ", err)
		return err
	}
	defer file.Close()

	// Запись строки в файл
	_, err = file.WriteString(content)
	if err != nil {
		fmt.Println("File write error:", err)
		return err
	}

	fmt.Println("File saved:", filePath)

	return nil
}

// checkDomain проверяет, соответствует ли домен URL заданному домену.
// @Improvement закэшить домен исходного урла
func checkDomain(doc *Document, domain string) bool {
	parsedURL, err := url.Parse(doc.URL)

	if err != nil {
		fmt.Println("Ошибка при разборе URL:", err)
		return false
	}

	domainURL, err := url.Parse(domain)

	if err != nil {
		fmt.Println("Ошибка при разборе URL:", err)
		return false
	}

	if parsedURL.Host == "" {
		parsedURL.Host = domainURL.Host
		parsedURL.Scheme = domainURL.Scheme
		doc.URL = parsedURL.String()
		fmt.Println("Relative link found", parsedURL.Path, doc.URL)
		return true // предполагаем, что относительные ссылки всегда в том же домене
	}

	// Сравниваем хост из разобранного URL с ожидаемым доменом
	return parsedURL.Host == domainURL.Host
}
