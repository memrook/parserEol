# Парсер каталога товаров с сайта stanki.ru

Парсер для извлечения информации о товарах с сайта [stanki.ru](https://www.stanki.ru/).

## Возможности

- Извлечение категорий товаров
- Извлечение информации о товарах (название, цена, описание, характеристики, изображения)
- Поддержка пагинации для загрузки всех товаров из категории
- Сохранение результатов в JSON-файл и CSV-файл с разделителем ";"
- Корректная обработка кириллицы (поддержка кодировки Windows-1251)
- Многопоточный парсинг с ограничением количества одновременных запросов
- Механизм повторных попыток при сетевых ошибках
- Режим исследования структуры сайта

## Требования

- Go 1.21 или выше
- Зависимости:
  - github.com/PuerkitoBio/goquery
  - golang.org/x/text/encoding
  - golang.org/x/text/encoding/charmap
  - golang.org/x/text/transform
  - golang.org/x/net/html/charset

## Установка

```bash
git clone <repository-url>
cd parserEol
go get -u github.com/PuerkitoBio/goquery
go get -u golang.org/x/text/encoding
go get -u golang.org/x/text/encoding/charmap
go get -u golang.org/x/text/transform
go get -u golang.org/x/net/html/charset
```

## Использование

### Запуск парсера

```bash
go run .
```

По умолчанию результаты будут сохранены в оба файла:
- `products.json` - для JSON формата
- `products.csv` - для CSV формата с разделителем ";"

### Выбор формата вывода

Можно указать формат вывода результатов:

```bash
# Только JSON
go run . -format json

# Только CSV
go run . -format csv

# Оба формата (по умолчанию)
go run . -format both
```

### Выбор категорий для парсинга

Можно указать конкретные категории для парсинга (через запятую):

```bash
go run . -categories="https://www.stanki.ru/catalog/metalloobrabatyvayuschee_oborudovanie/,https://www.stanki.ru/catalog/derevoobrabatyvayushhee_oborudovanie/"
```

Например, для парсинга основных категорий, содержащих большинство товаров:

```bash
go run . -categories="https://www.stanki.ru/catalog/metalloobrabatyvayuschee_oborudovanie/,https://www.stanki.ru/catalog/derevoobrabatyvayushhee_oborudovanie/,https://www.stanki.ru/catalog/instrument/,https://www.stanki.ru/catalog/oborudovanie_dlya_proizvodstva_mebeli/,https://www.stanki.ru/catalog/tyazhelaya_metalloobrabotka/"
```

### Ограничение количества категорий

Для тестирования или ограничения объема данных можно указать максимальное количество категорий для парсинга:

```bash
go run . -limit 5
```

### Пропуск загрузки детальной информации

Для ускорения работы парсера можно пропустить загрузку детальной информации о товарах (описания и характеристики со страницы товара):

```bash
go run . -skip-details
```

### Указание диапазона страниц

Для парсинга определенного диапазона страниц в категориях:

```bash
# Начать с 2-й страницы
go run . -start-page 2

# Обработать только страницы с 3-й по 5-ю
go run . -start-page 3 -end-page 5
```

### Режим исследования пагинации

Для анализа пагинации на конкретной странице:

```bash
go run . -inspect-pagination -categories="https://www.stanki.ru/catalog/metalloobrabatyvayuschee_oborudovanie/"
```

Результаты анализа будут сохранены в файл `pagination_structure.txt`.

### Режим исследования структуры сайта

Для анализа HTML-структуры сайта можно использовать режим исследования:

```bash
go run . -inspect
```

Результаты анализа будут сохранены в файлы `catalog_structure.txt` и `category_structure.txt`.

## Особенности

### Пагинация

Парсер поддерживает пагинацию каталога и загружает товары со всех страниц категории. Для этого он анализирует наличие кнопок "Следующая" или соответствующих элементов навигации и добавляет к URL параметр `?PAGEN_2=N`, где N - номер страницы.

### Обогащение товаров детальной информацией

Парсер может загружать детальную информацию о товаре (описание и характеристики) с индивидуальной страницы товара. Эта функциональность может быть отключена с помощью флага `-skip-details` для ускорения работы.

### Поддержка кириллицы

Парсер корректно обрабатывает и сохраняет кириллические символы в выходных файлах (JSON и CSV). Для правильного отображения в Windows используется маркер BOM (Byte Order Mark) в начале файлов.

При открытии файлов в текстовом редакторе или Excel рекомендуется использовать кодировку UTF-8.

```powershell
# Просмотр файлов с корректной кодировкой в PowerShell
Get-Content -Path products.json -Encoding UTF8
Get-Content -Path products.csv -Encoding UTF8
```

## Структура проекта

- `main.go` - основной файл с парсером
- `inspect.go` - код для исследования структуры сайта
- `products.json` - результаты парсинга в формате JSON
- `products.csv` - результаты парсинга в формате CSV

## Настройка

В файле `main.go` можно настроить следующие параметры:

- `baseURL` - базовый URL сайта
- `catalogURL` - URL каталога товаров
- `concurrency` - количество одновременных запросов
- `delay` - задержка между запросами в миллисекундах

Также можно настроить параметры механизма повторных попыток в функции `doRequestWithRetry`:
- Для получения категорий: 3 попытки
- Для получения товаров и деталей товара: 2 попытки

## Лицензия

MIT 