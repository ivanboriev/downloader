package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"
)

func main() {

	var (
		help bool
		h    bool
	)

	flag.BoolVar(&help, "help", false, "Показать помощь")
	flag.BoolVar(&h, "h", false, "Показать помощь")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `downloader — утилита для загрузки файлов

Использование:
downloader <директория> <url1> [url2...]
--help, -h Помощь

Примеры:
downloader ./downloads http://example.com/file1.zip http://example.com/file2.zip
			
`)

	}

	flag.Parse()

	if help || h || len(os.Args) < 3 {
		flag.Usage()
		os.Exit(2)
	}

	directory := os.Args[1]
	urls := os.Args[2:]

	fmt.Println("Директория сохранения: ", directory)
	fmt.Println("URL для скачивания:")

	for _, url := range urls {
		fmt.Println("  - ", url)
	}

	var wg sync.WaitGroup

	for _, url := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			fmt.Printf("Начинаем скачивание: %s\n", url)
			if err := downloadFile(url, directory); err != nil {
				fmt.Fprintf(os.Stderr, "Ошибка при скачивании %s: %v\n", url, err)
			} else {
				fmt.Printf("Файл %s успешно скачан.\n", url)
			}
		}(url)
	}

	wg.Wait()

	fmt.Println("Загрузка завершена.")
}

func downloadFile(url, savePath string) error {
	if err := os.MkdirAll(savePath, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", savePath, err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http get %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s for %s", resp.Status, url)
	}

	filename := path.Base(url)
	savePath = filepath.Join(savePath, filename)

	out, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("create file %s: %w", savePath, err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("write file %s: %w", savePath, err)
	}

	return nil
}
