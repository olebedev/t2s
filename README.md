# t2s - text-to-speech tool for russian language

Преобразование текста в звук при помощи Yandex SpeechKit Cloud

### Использование
Установка: `go get github.com/olebedev/t2s`.

```
$ t2s -h
NAME:
   t2s - преобразование русского текста в звук при помощи Yandex SpeechKit Cloud

USAGE:
   t2s --key <API access key> [options]

VERSION:
   0.1.0

AUTHOR(S):
   olebedev <ole6edev@gmail.com>

COMMANDS:
GLOBAL OPTIONS:
   --key, -k                    API ключ
   --limit, -l "100"            лимит на количество одновременных запросов
   --timeout, -t "120"          таймаут для http клиента, сек
   --attempts, -a "5"           количество попыток запроса конвертации одного чанка
   --output, -o                 целевой файл, по умолчанию вывод делается в stdout
   --input, -i                  файл с текстом, по умолчанию берется из stdin
   --speaker, -s "zahar"        голос, варианты: [jane, omazh, zahar, ermil]
   --emotion, -e "good"         эмоции, варианты: [good, neutral, evil, mixed]
   --help, -h                   show help
   --version, -v                print the version
```

Пример: `t2s -k <Yandex API key> -i text.txt -o result.mp3`
