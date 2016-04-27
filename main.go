package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blang/vfs/memfs"
	"github.com/codegangsta/cli"
	"github.com/dmulholland/mp3cat/mp3lib"
	"github.com/gosuri/uiprogress"
)

const (
	maxChunkLength = 870
	fileType       = "mp3"
	lang           = "ru-RU"
	targetURL      = "https://tts.voicetech.yandex.net/generate"
)

func main() {
	app := cli.NewApp()
	app.Usage = "преобразование русского текста в звук при помощи Yandex SpeechKit Cloud"
	app.UsageText = "t2s --key <API access key> [options]"
	app.ArgsUsage = "- -2 -3"
	app.Author = "olebedev"
	app.Email = "ole6edev@gmail.com"
	app.Version = "0.1.0"
	app.Action = do
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "key,k",
			Usage: "API ключ",
		},
		cli.IntFlag{
			Name:  "limit,l",
			Usage: "лимит на количество одновременных запросов",
			Value: 100,
		},
		cli.IntFlag{
			Name:  "timeout,t",
			Usage: "таймаут для http клиента, сек",
			Value: 120,
		},
		cli.IntFlag{
			Name:  "attempts,a",
			Usage: "количество попыток запроса конвертации одного чанка",
			Value: 5,
		},
		cli.StringFlag{
			Name:  "output,o",
			Usage: "целевой файл, по умолчанию вывод делается в stdout",
		},
		cli.StringFlag{
			Name:  "input,i",
			Usage: "файл с текстом, по умолчанию берется из stdin",
		},
		cli.StringFlag{
			Name:  "speaker,s",
			Usage: "голос, варианты: [jane, omazh, zahar, ermil]",
			Value: "zahar",
		},
		cli.StringFlag{
			Name:  "emotion,e",
			Usage: "эмоции, варианты: [good, neutral, evil, mixed]",
			Value: "good",
		},
	}
	app.Run(os.Args)
}

func do(c *cli.Context) {
	if c.String("key") == "" {
		log.Fatalln("Ключ API не найден!")
	}

	var reader io.Reader
	if c.String("input") == "" {
		reader = bufio.NewReader(os.Stdin)
	} else {
		filename, err := filepath.Abs(c.String("input"))
		must(err)
		f, err := os.Open(filename)
		must(err)
		defer f.Close()
		reader = f
	}

	input, err := ioutil.ReadAll(reader)
	must(err)
	splitted, err := split(string(input))
	must(err)

	files, err := getFiles(
		splitted,
		c.String("key"),
		c.String("speaker"),
		c.String("emotion"),
		c.Int("limit"),
		c.String("output") != "",
		time.Duration(c.Int("timeout"))*time.Second,
		c.Int("attempts"),
	)
	must(err)
	large, err := concat(files)
	must(err)
	l := bytes.NewBuffer(large)
	if c.String("output") == "" {
		l.WriteTo(os.Stdout)
	} else {
		filename, err := filepath.Abs(c.String("output"))
		must(err)
		out, err := os.Create(filename)
		defer out.Close()
		must(err)
		l.WriteTo(out)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func split(s string) ([]string, error) {
	var ret []string
	splitted := strings.SplitAfter(s, " ")
	index := 0
	length := len(splitted)

Main:
	for {
		l := 0
		if index >= length {
			break Main
		}
		var chunk []string
	Sub:
		for ; index < length; index++ {
			current := splitted[index]
			if l+len(current) > maxChunkLength {
				break Sub
			}
			chunk = append(chunk, current)
			l += len(current)
		}
		ret = append(ret, strings.Join(chunk, ""))
	}
	return ret, nil
}

func getFiles(
	splitted []string,
	key, speaker, emotion string,
	limit int, showProgress bool,
	timeout time.Duration,
	attempts int,
) ([][]byte, error) {
	type resp struct {
		err   error
		index int
		body  []byte
	}
	client := http.Client{Timeout: timeout}
	acc := make([][]byte, len(splitted))
	consume := make(chan *resp)
	semaphore := make(chan struct{}, limit)

	bar := uiprogress.AddBar(len(splitted)).AppendCompleted().PrependElapsed()
	if showProgress {
		uiprogress.Start()
	}

	for i, s := range splitted {
		go func(index int, s string) {
			semaphore <- struct{}{}
			params := url.Values{}
			params.Set("text", s)
			params.Set("format", fileType)
			params.Set("lang", lang)
			params.Set("speaker", speaker)
			params.Set("key", key)
			params.Set("emotion", emotion)

			var res *http.Response
			var err error
			for i := 0; i < attempts; i++ {
				res, err = client.Get(targetURL + "?" + params.Encode())
				if err != nil && strings.HasSuffix(err.Error(), "(Client.Timeout exceeded while awaiting headers)") {
					fmt.Fprintf(os.Stderr, "got timeout error for chunk with index: %d", index)
				} else {
					break
				}
			}

			bar.Incr()
			if err != nil {
				consume <- &resp{err: err, index: index}
				return
			}
			defer res.Body.Close()

			d, err := ioutil.ReadAll(res.Body)
			if err != nil {
				consume <- &resp{err: err, index: index}
				return
			}

			if res.StatusCode != 200 {
				err = errors.New(string(d))
			}

			consume <- &resp{body: d, err: err, index: index}
		}(i, s)
	}

	for range splitted {
		r := <-consume
		<-semaphore
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "http error: %v, text: %s", r.err, splitted[r.index])
			acc[r.index] = make([]byte, 0)
			continue
			// return acc, r.err
		}
		acc[r.index] = r.body
	}

	return acc, nil
}

func concat(files [][]byte) ([]byte, error) {
	var totalFrames uint32
	var totalBytes uint32
	var totalFiles int
	var firstBitRate int
	var isVBR bool

	fs := memfs.Create()
	outputFile, err := fs.OpenFile("/tmp.mp3", os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		inputFile := bytes.NewReader(file)
		isFirstFrame := true

		for {
			frame := mp3lib.NextFrame(inputFile)
			if frame == nil {
				break
			}

			if isFirstFrame {
				isFirstFrame = false
				if mp3lib.IsXingHeader(frame) || mp3lib.IsVbriHeader(frame) {
					continue
				}
			}

			if firstBitRate == 0 {
				firstBitRate = frame.BitRate
			} else if frame.BitRate != firstBitRate {
				isVBR = true
			}

			_, err = outputFile.Write(frame.RawBytes)
			if err != nil {
				return nil, err
			}

			totalFrames++
			totalBytes += uint32(len(frame.RawBytes))
		}

		totalFiles++
	}

	outputFile.Seek(0, os.SEEK_SET)
	b, err := ioutil.ReadAll(outputFile)
	outputFile.Close()

	// TODO prepend an Xing VBR header
	_ = isVBR
	return b, err
}
