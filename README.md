# t2s - text-to-speech tool for russian language

Преобразование текста в звук при помощи Yandex SpeechKit Cloud

### Использование
Установка: `go get github.com/olebedev/t2s`.

```
$ t2s -h
NAME:
   t2s - преобразование русского текста в звук при помощи Yandex SpeechKit Cloud

USAGE:
   t2s [global options] command [command options] [arguments...]

VERSION:
   0.1.0

COMMANDS:
GLOBAL OPTIONS:
   --key, -k      API ключ
   --output, -o     целевой файл, по умолчанию вывод делается в stdout
   --input, -i      файл с текстом, по умолчанию берется из stdin
   --speaker, -s "zahar"  голос, варианты: [jane, omazh, zahar, ermil]
   --emotion, -e "good"   эмоции, варианты: [good, neutral, evil, mixed]
   --help, -h     show help
   --version, -v    print the version
```

Пример: `cat text.txt | t2t -k <Yandex API key> > result.mp3`
