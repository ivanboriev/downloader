package main

import (
	"encoding/json"
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

const chunkSize = 10 * 1024 * 1024 // 10 MB

type Chunk struct {
	Index int
	Start int64
	End   int64
}

type Headers struct {
	ContentLength int64
	AcceptRanges  bool
}

type Downloader struct {
	Client    *http.Client
	Directory string
	Urls      []string
}

type DownloadState struct {
	URL              string `json:"url"`
	TotalSize        int64  `json:"total_size"`
	ChunkSize        int    `json:"chunk_size"`
	TotalChunks      int    `json:"total_chunks"`
	DownloadedChunks []bool `json:"downloaded_chunks"`
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

func splitIntoChunks(fileSize int64) []Chunk {
	var chunks []Chunk
	var start int64

	for i := 0; start < fileSize; i++ {
		end := start + chunkSize - 1
		if end >= fileSize {
			end = fileSize - 1
		}
		chunks = append(chunks, Chunk{
			Index: i,
			Start: start,
			End:   end,
		})
		start = end + 1
	}

	return chunks
}

func (d *Downloader) Process() {
	var wg sync.WaitGroup

	for _, url := range d.Urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			fmt.Printf("Фaйл: %s\n", path.Base(url))
			if headers, err := d.GetHeaders(url); err != nil {
				fmt.Fprintf(os.Stderr, "Ошибка при получении заголовков %s: %v\n", url, err)
			} else {
				fmt.Printf("Размер файла: %d байт\n", headers.ContentLength)
				fmt.Printf("Поддержка докачки: %t\n", headers.AcceptRanges)

				if headers.AcceptRanges && headers.ContentLength > 0 {
					chunks := splitIntoChunks(headers.ContentLength)
					fmt.Printf("Количество чанков: %d\n", len(chunks))

					filename := path.Base(url)
					savePath := filepath.Join(d.Directory, filename)

					state := DownloadState{
						URL:              url,
						TotalSize:        headers.ContentLength,
						ChunkSize:        chunkSize,
						TotalChunks:      len(chunks),
						DownloadedChunks: make([]bool, len(chunks)),
					}
					stateData, err := json.MarshalIndent(state, "", "  ")
					if err != nil {
						fmt.Fprintf(os.Stderr, "Ошибка при сериализации состояния: %v\n", err)
						return
					}
					progressPath := savePath + ".progress"
					if err := os.WriteFile(progressPath, stateData, 0644); err != nil {
						fmt.Fprintf(os.Stderr, "Ошибка при записи файла состояния %s: %v\n", progressPath, err)
						return
					}

					out, err := os.Create(savePath)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Ошибка при создании файла %s: %v\n", savePath, err)
						return
					}
					if err := out.Truncate(headers.ContentLength); err != nil {
						fmt.Fprintf(os.Stderr, "Ошибка при установке размера файла %s: %v\n", savePath, err)
						out.Close()
						return
					}
					out.Close()

					out, err = os.OpenFile(savePath, os.O_WRONLY|os.O_APPEND, 0644)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Ошибка при открытии файла %s: %v\n", savePath, err)
						return
					}

					for index, chunk := range chunks {
						fmt.Printf("Скачиваю чанк %d: %d-%d\n", chunk.Index, chunk.Start, chunk.End)

						req, err := http.NewRequest("GET", url, nil)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Ошибка при создании запроса для чанка %d: %v\n", chunk.Index, err)
							defer out.Close()
							return
						}
						req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", chunk.Start, chunk.End))

						resp, err := d.Client.Do(req)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Ошибка при скачивании чанка %d: %v\n", chunk.Index, err)
							defer out.Close()
							return
						}

						if resp.StatusCode != http.StatusPartialContent {
							fmt.Fprintf(os.Stderr, "Неожиданный статус для чанка %d: %s\n", chunk.Index, resp.Status)
							defer resp.Body.Close()
							defer out.Close()
							return
						}

						out.Seek(chunk.Start, io.SeekStart)

						_, err = io.Copy(out, resp.Body)

						state.DownloadedChunks[index] = true

						stateData, err := json.MarshalIndent(state, "", "  ")
						if err != nil {
							fmt.Fprintf(os.Stderr, "Ошибка при сериализации состояния: %v\n", err)
							return
						}
						progressPath := savePath + ".progress"
						if err := os.WriteFile(progressPath, stateData, 0644); err != nil {
							fmt.Fprintf(os.Stderr, "Ошибка при записи файла состояния %s: %v\n", progressPath, err)
							return
						}

						defer resp.Body.Close()
						if err != nil {
							fmt.Fprintf(os.Stderr, "Ошибка при записи чанка %d: %v\n", chunk.Index, err)
							defer out.Close()
							return
						}

						fmt.Printf("Чанк %d скачан.\n", chunk.Index)
					}

					defer out.Close()
					fmt.Printf("Файл %s успешно скачан.\n", url)
					return
				}
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
