package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type Headers struct {
	ContentLength int64
	AcceptRanges  bool
}

type Downloader struct {
	Client    *http.Client
	Directory string
	Urls      []string
}

func NewDownloader(directory string, urls []string) *Downloader {
	return &Downloader{
		Client:    &http.Client{Timeout: 30 * time.Second},
		Directory: directory,
		Urls:      urls,
	}
}

func (d *Downloader) Download(url, savePath string) error {
	if err := os.MkdirAll(savePath, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", savePath, err)
	}

	resp, err := d.Client.Get(url)
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

func (d *Downloader) GetHeaders(url string) (Headers, error) {
	resp, err := d.Client.Head(url)
	if err != nil {
		return Headers{}, err
	}
	defer resp.Body.Close()

	// Размер файла
	contentLength := resp.Header.Get("Content-Length")
	size, _ := strconv.ParseInt(contentLength, 10, 64)

	// Поддержка докачки
	acceptRanges := resp.Header.Get("Accept-Ranges")
	supportsResume := acceptRanges == "bytes"

	return Headers{
		ContentLength: size,
		AcceptRanges:  supportsResume,
	}, nil
}

func (d *Downloader) Process() {
	var wg sync.WaitGroup

	for _, url := range d.Urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			fmt.Printf("Фaйл: %s", path.Base(url))
			if headers, err := d.GetHeaders(url); err != nil {
				fmt.Fprintf(os.Stderr, "Ошибка при получении заголовков %s: %v\n", url, err)
			} else {
				fmt.Printf("Размер файла: %d байт\n", headers.ContentLength)
				fmt.Printf("Поддержка докачки: %t\n", headers.AcceptRanges)
			}

			fmt.Printf("Начинаем скачивание: %s\n", url)
			if err := d.Download(url, d.Directory); err != nil {
				fmt.Fprintf(os.Stderr, "Ошибка при скачивании %s: %v\n", url, err)
			} else {
				fmt.Printf("Файл %s успешно скачан.\n", url)
			}
		}(url)
	}

	wg.Wait()

}

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
	fmt.Println("URL для скачивания:", urls)

	dl := NewDownloader(directory, urls)

	dl.Process()

	fmt.Println("Загрузка завершена.")
}
