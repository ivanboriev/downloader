package main

import (
	"flag"
	"fmt"
	"os"
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

	args := flag.Args()

	if help || h || len(args) < 3 {
		flag.Usage()
		os.Exit(2)
	}

	directory := args[0]
	urls := args[1:]

	fmt.Println("Директория сохранения: ", directory)
	fmt.Println("URL для скачивания:")

	for _, url := range urls {
		fmt.Println("  - ", url)
	}

}
