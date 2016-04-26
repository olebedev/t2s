package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/blang/vfs/memfs"
	"github.com/codegangsta/cli"
	"github.com/dmulholland/mp3cat/mp3lib"
	"github.com/gosuri/uiprogress"
)

const (
	MAX_CHUNK_LENGTH = 870
	FILE_TYPE        = "mp3"
	LANG             = "ru-RU"
	URL              = "https://tts.voicetech.yandex.net/generate"
)

func main() {
	app := cli.NewApp()
	app.Usage = "преобразование русского текста в звук при помощи Yandex SpeechKit Cloud"
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
	ret := make([]string, 0)
	splitted := strings.SplitAfter(s, " ")
	index := 0
	length := len(splitted)

Main:
	for {
		l := 0
		if index >= length {
			break Main
		}
		chunk := make([]string, 0)
	Sub:
		for ; index < length; index++ {
			current := splitted[index]
			if l+len(current) > MAX_CHUNK_LENGTH {
				break Sub
			}
			chunk = append(chunk, current)
			l += len(current)
		}
		ret = append(ret, strings.Join(chunk, ""))
	}
	return ret, nil
}

func getFiles(splitted []string, key, speaker, emotion string, limit int, showProgress bool) ([][]byte, error) {
	type resp struct {
		err   error
		index int
		body  []byte
	}

	acc := make([][]byte, len(splitted))
	consume := make(chan *resp)
	semaphore := make(chan struct{}, limit)

	bar := uiprogress.AddBar(len(splitted)).AppendCompleted().PrependElapsed()
	if showProgress {
		// start rendering
		uiprogress.Start()
	}

	for i, s := range splitted {
		go func(index int, s string, c chan *resp) {
			semaphore <- struct{}{}
			params := url.Values{}
			params.Set("text", s)
			params.Set("format", FILE_TYPE)
			params.Set("lang", LANG)
			params.Set("speaker", speaker)
			params.Set("key", key)
			params.Set("emotion", emotion)

			res, err := http.Get(URL + "?" + params.Encode())
			bar.Incr()
			if err != nil {
				c <- &resp{err: err, index: index}
				return
			}
			defer res.Body.Close()

			d, err := ioutil.ReadAll(res.Body)
			if err != nil {
				c <- &resp{err: err, index: index}
				return
			}

			if res.StatusCode != 200 {
				err = errors.New(string(d))
			}

			c <- &resp{body: d, err: err, index: index}
		}(i, s, consume)
	}

	for range splitted {
		r := <-consume
		<-semaphore
		if r.err != nil {
			return acc, r.err
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

			_, err := outputFile.Write(frame.RawBytes)
			if err != nil {
				return nil, err
			}

			totalFrames += 1
			totalBytes += uint32(len(frame.RawBytes))
		}

		totalFiles += 1
	}

	outputFile.Seek(0, os.SEEK_SET)
	b, err := ioutil.ReadAll(outputFile)
	outputFile.Close()

	// TODO prepend an Xing VBR header
	_ = isVBR
	return b, err
}
