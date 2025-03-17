package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"io"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/cases"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/language"
	"golang.org/x/text/transform"
)

// Product представляет собой товар из каталога
type Product struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Description string   `json:"description"`
	Price       string   `json:"price"`
	ImageURL    string   `json:"image_url"`
	Category    string   `json:"category"`
	Features    []string `json:"features"`
}

// Category представляет собой категорию товаров
type Category struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

const (
	baseURL     = "https://www.stanki.ru"
	catalogURL  = "https://www.stanki.ru/catalog/"
	concurrency = 5   // Количество одновременных запросов
	delay       = 500 // Задержка между запросами в миллисекундах
)

var (
	client = &http.Client{
		Timeout: time.Second * 30,
	}
)

func main() {
	// Флаг для выбора режима работы
	inspectMode := flag.Bool("inspect", false, "Запустить в режиме исследования структуры сайта")
	inspectPagination := flag.Bool("inspect-pagination", false, "Запустить в режиме исследования пагинации")
	limitCategories := flag.Int("limit", 0, "Ограничить количество категорий для парсинга (0 - без ограничений)")
	outputFormat := flag.String("format", "both", "Формат вывода: json, csv или both (и то, и другое)")
	skipDetails := flag.Bool("skip-details", false, "Пропустить загрузку детальной информации о товарах")
	categoryURLs := flag.String("categories", "", "Список URL категорий через запятую (если не указано, будут использованы все категории)")
	startPage := flag.Int("start-page", 1, "Начальная страница для парсинга (по умолчанию 1)")
	endPage := flag.Int("end-page", 0, "Конечная страница для парсинга (0 - все страницы)")
	threads := flag.Int("threads", concurrency, "Количество одновременных потоков для загрузки данных (по умолчанию 5)")
	enrichThreads := flag.Int("enrich-threads", 10, "Количество одновременных потоков для обогащения деталями (по умолчанию 10)")
	delayMs := flag.Int("delay", delay, "Задержка между запросами в миллисекундах (по умолчанию 500)")
	flag.Parse()

	// Обновляем значения задержки, если указано в параметрах
	if *delayMs != delay {
		log.Printf("Установлена задержка между запросами: %d мс", *delayMs)
	}

	if *inspectMode {
		fmt.Println("Запуск в режиме исследования структуры сайта...")
		inspectMain()
		return
	}

	if *inspectPagination {
		fmt.Println("Запуск в режиме исследования пагинации...")

		// Проверяем, указана ли категория
		if *categoryURLs == "" {
			log.Fatal("Для исследования пагинации необходимо указать URL категории через параметр -categories")
		}

		// Берем первую категорию из списка
		url := strings.Split(*categoryURLs, ",")[0]
		url = strings.TrimSpace(url)

		inspectPaginationOnCategory(url)
		return
	}

	fmt.Println("Начинаем парсинг каталога товаров с сайта stanki.ru")

	var categories []Category
	var err error

	// Если указаны конкретные категории, используем их
	if *categoryURLs != "" {
		// Разбиваем строку с URL категорий на отдельные URL
		urls := strings.Split(*categoryURLs, ",")

		// Преобразуем URL в категории
		for _, url := range urls {
			url = strings.TrimSpace(url)
			if url == "" {
				continue
			}

			// Получаем название категории из URL
			parts := strings.Split(url, "/")
			var name string
			if len(parts) > 0 {
				// Берем последний непустой элемент как название
				for i := len(parts) - 1; i >= 0; i-- {
					if parts[i] != "" {
						name = parts[i]
						name = strings.ReplaceAll(name, "_", " ")
						name = cases.Title(language.Russian).String(name)
						break
					}
				}
			}

			// Добавляем категорию
			categories = append(categories, Category{
				Name: name,
				URL:  url,
			})

			fmt.Printf("Добавлена пользовательская категория: %s (%s)\n", name, url)
		}
	} else {
		// Получаем категории с сайта
		categories, err = getCategories()
		if err != nil {
			log.Fatalf("Ошибка получения категорий: %v", err)
		}
	}

	// Ограничиваем количество категорий, если указан лимит
	if *limitCategories > 0 && *limitCategories < len(categories) {
		fmt.Printf("Ограничиваем парсинг до %d категорий из %d\n", *limitCategories, len(categories))
		categories = categories[:*limitCategories]
	}

	fmt.Printf("Найдено %d категорий\n", len(categories))

	// Канал для сбора всех товаров
	productChan := make(chan Product)

	// WaitGroup для ожидания завершения всех горутин
	var wg sync.WaitGroup

	// Семафор для ограничения количества одновременных запросов
	semaphore := make(chan struct{}, *threads)

	// Запускаем парсинг каждой категории в отдельной горутине
	for _, category := range categories {
		wg.Add(1)
		go func(cat Category) {
			defer wg.Done()
			products, err := getProductsFromCategory(cat, semaphore, *startPage, *endPage, *delayMs)
			if err != nil {
				log.Printf("Ошибка парсинга категории %s: %v", cat.Name, err)
				return
			}

			for _, product := range products {
				productChan <- product
			}
		}(category)
	}

	// Горутина для закрытия канала после завершения всех парсеров
	go func() {
		wg.Wait()
		close(productChan)
	}()

	// Собираем все товары в массив
	var allProducts []Product
	for product := range productChan {
		allProducts = append(allProducts, product)
	}

	fmt.Printf("Всего найдено %d товаров\n", len(allProducts))

	// Удаляем дубликаты товаров по ID
	allProducts = removeDuplicateProducts(allProducts)
	fmt.Printf("После удаления дубликатов: %d уникальных товаров\n", len(allProducts))

	// Если не нужно пропускать детали, обогащаем товары детальной информацией
	if !*skipDetails {
		fmt.Println("Начинаем обогащение товаров детальной информацией...")
		// Создаем новый слайс для обогащенных товаров
		// и передаем его по ссылке
		enrichedProducts := make([]Product, len(allProducts))
		copy(enrichedProducts, allProducts)

		// Создаем отдельный семафор для обогащения с возможно большим количеством потоков
		enrichSemaphore := make(chan struct{}, *enrichThreads)
		log.Printf("Используется %d одновременных потоков для обогащения", *enrichThreads)

		enrichProductsWithDetails(enrichedProducts, enrichSemaphore, *delayMs)
		// Заменяем исходный слайс обогащенным
		allProducts = enrichedProducts
		fmt.Println("Обогащение товаров завершено")
	} else {
		fmt.Println("Пропуск загрузки детальной информации о товарах (флаг -skip-details)")
	}

	// Сохраняем результаты в выбранном формате
	saveOutput := func(format string) {
		switch format {
		case "json", "both":
			// Сохраняем результаты в JSON файл
			err = saveToJSON(allProducts, "products.json")
			if err != nil {
				log.Printf("Ошибка при сохранении в JSON: %v", err)
			} else {
				fmt.Println("Результаты сохранены в файл products.json")
			}
		}

		switch format {
		case "csv", "both":
			// Сохраняем результаты в CSV файл
			err = saveToCSV(allProducts, "products.csv")
			if err != nil {
				log.Printf("Ошибка при сохранении в CSV: %v", err)
			} else {
				fmt.Println("Результаты сохранены в файл products.csv")
			}
		}
	}

	saveOutput(strings.ToLower(*outputFormat))
	fmt.Println("Парсинг завершен.")
}

// doRequestWithRetry выполняет HTTP запрос с повторными попытками в случае ошибки
func doRequestWithRetry(url string, maxRetries int, delayMs int) (*http.Response, error) {
	var resp *http.Response
	var err error

	for i := 0; i < maxRetries; i++ {
		resp, err = client.Get(url)
		if err == nil {
			return resp, nil
		}

		log.Printf("Ошибка при запросе %s: %v. Повторная попытка %d из %d", url, err, i+1, maxRetries)
		time.Sleep(time.Duration(delayMs*(i+1)) * time.Millisecond) // Увеличиваем задержку с каждой попыткой
	}

	return nil, fmt.Errorf("не удалось выполнить запрос после %d попыток: %v", maxRetries, err)
}

// getCategories получает список всех категорий с сайта
func getCategories() ([]Category, error) {
	resp, err := doRequestWithRetry(catalogURL, 3, delay)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка при получении страницы каталога: %d", resp.StatusCode)
	}

	// Определяем кодировку и создаем Reader с преобразованием в UTF-8
	utf8Reader, err := getUTF8Reader(resp.Body)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(utf8Reader)
	if err != nil {
		return nil, err
	}

	var categories []Category

	// Ищем категории по селектору на основе результатов анализа
	// Выбираем ссылки внутри блока каталога
	doc.Find("a[href^='/catalog/']").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		// Фильтруем технические URL и страницы конкретных товаров
		if strings.Contains(href, "_") && !strings.Contains(href, ".html") {
			name := strings.TrimSpace(s.Text())
			if name != "" && len(name) < 100 { // Проверка на валидность имени
				categories = append(categories, Category{
					Name: name,
					URL:  baseURL + href,
				})
			}
		}
	})

	// Удаляем дубликаты категорий
	uniqueCategories := make([]Category, 0)
	seen := make(map[string]bool)

	for _, category := range categories {
		if !seen[category.URL] {
			seen[category.URL] = true
			uniqueCategories = append(uniqueCategories, category)
		}
	}

	return uniqueCategories, nil
}

// getProductsFromCategory получает все товары из указанной категории
func getProductsFromCategory(category Category, semaphore chan struct{}, startPage, endPage int, delayMs int) ([]Product, error) {
	semaphore <- struct{}{}        // Занимаем слот в семафоре
	defer func() { <-semaphore }() // Освобождаем слот при выходе

	var allProducts []Product
	pageNum := startPage
	maxPages := 100 // Ограничение на максимальное количество страниц

	// Если указана конечная страница, используем её
	if endPage > 0 && endPage < maxPages {
		maxPages = endPage
	}

	// Обрабатываем все страницы категории
	for pageNum <= maxPages {
		// Формируем URL с учетом пагинации
		pageURL := category.URL
		if pageNum > 1 {
			if strings.Contains(pageURL, "?") {
				pageURL += "&PAGEN_2=" + fmt.Sprintf("%d", pageNum)
			} else {
				pageURL += "?PAGEN_2=" + fmt.Sprintf("%d", pageNum)
			}
		}

		log.Printf("Обрабатываем страницу %d категории %s: %s", pageNum, category.Name, pageURL)

		// Делаем задержку между запросами страниц
		time.Sleep(time.Duration(delayMs) * time.Millisecond)

		// Получаем страницу с товарами
		resp, err := doRequestWithRetry(pageURL, 2, delayMs)
		if err != nil {
			return nil, err
		}

		// Определяем кодировку и создаем Reader с преобразованием в UTF-8
		utf8Reader, err := getUTF8Reader(resp.Body)
		if err != nil {
			resp.Body.Close()
			return nil, err
		}

		doc, err := goquery.NewDocumentFromReader(utf8Reader)
		resp.Body.Close() // Закрываем Body после использования

		if err != nil {
			return nil, err
		}

		// Ищем товары на текущей странице
		products, hasNextPage := extractProductsFromPage(doc, category)

		// Добавляем товары в общий список
		allProducts = append(allProducts, products...)

		log.Printf("Найдено %d товаров на странице %d категории %s (всего: %d)",
			len(products), pageNum, category.Name, len(allProducts))

		// Если нет кнопки следующей страницы или не найдено товаров, прекращаем обработку
		if !hasNextPage || len(products) == 0 {
			break
		}

		pageNum++
	}

	return allProducts, nil
}

// extractProductsFromPage извлекает товары с текущей страницы и проверяет наличие следующей страницы
func extractProductsFromPage(doc *goquery.Document, category Category) ([]Product, bool) {
	var products []Product

	// Ищем товары по селектору на основе результатов анализа
	doc.Find("[data-product-id]").Each(func(i int, s *goquery.Selection) {
		// Извлекаем ID товара
		productID, exists := s.Attr("data-product-id")
		if !exists {
			return
		}

		// Извлекаем название товара
		nameElement := s.Find(".productCard__name")
		name := strings.TrimSpace(nameElement.Text())

		// Извлекаем URL товара
		url, exists := nameElement.Attr("href")
		if !exists {
			return
		}

		// Извлекаем цену товара
		price := strings.TrimSpace(s.Find(".productCard__price").Text())

		// Извлекаем URL изображения товара
		imgURL := ""
		s.Find(".productCard__preview img").Each(func(j int, img *goquery.Selection) {
			if j == 0 { // Берем только первое изображение
				src, exists := img.Attr("src")
				if exists {
					imgURL = src
				}
			}
		})

		// Извлекаем параметры товара
		var features []string
		s.Find(".productCard__params p").Each(func(j int, p *goquery.Selection) {
			feature := strings.TrimSpace(p.Text())
			if feature != "" {
				features = append(features, feature)
			}
		})

		product := Product{
			ID:       productID,
			Name:     name,
			URL:      baseURL + url,
			Price:    price,
			ImageURL: baseURL + imgURL,
			Category: category.Name,
			Features: features,
		}

		// Не загружаем детальную информацию здесь, чтобы ускорить парсинг
		// Детальная информация будет загружаться отдельно при необходимости

		products = append(products, product)
	})

	// Специфичные для сайта селекторы пагинации
	paginationSelectors := []string{
		".pagination", ".paginations", ".nav-links", ".pager",
		".pages", ".pagenation", ".modern-page-navigation",
	}

	// Проверяем наличие следующей страницы
	hasNextPage := false

	// 1. Проверяем наличие кнопок пагинации с data-pagination-button или data-pagination-more
	doc.Find("[data-pagination-button], [data-pagination-more]").Each(func(i int, s *goquery.Selection) {
		// Проверяем атрибуты
		for _, attr := range []string{"data-pagination-button", "data-pagination-more"} {
			href, exists := s.Attr(attr)
			if exists && strings.Contains(href, "PAGEN_2=") {
				hasNextPage = true
				return
			}
		}

		// Проверяем класс кнопки "Следующая"
		class, _ := s.Attr("class")
		disabled, _ := s.Attr("disabled")
		if strings.Contains(class, "button_next") && disabled == "" {
			hasNextPage = true
			return
		}
	})

	// 2. Ищем элементы пагинации
	if !hasNextPage {
		for _, selector := range paginationSelectors {
			paginationElement := doc.Find(selector)
			if paginationElement.Length() > 0 {
				// Ищем внутри пагинации ссылки на следующую страницу
				paginationElement.Find("a, span, div, button").Each(func(i int, s *goquery.Selection) {
					text := strings.ToLower(strings.TrimSpace(s.Text()))
					class, _ := s.Attr("class")
					href, hrefExists := s.Attr("href")

					// Проверяем, не отключена ли кнопка
					disabled, _ := s.Attr("disabled")
					if disabled != "" {
						return
					}

					// Проверяем текст, класс или href ссылки
					if strings.Contains(text, "след") ||
						strings.Contains(text, "next") ||
						strings.Contains(text, "показать еще") ||
						strings.Contains(class, "next") ||
						strings.Contains(class, "button_next") ||
						strings.Contains(class, "modern-page-next") ||
						(hrefExists && strings.Contains(href, "PAGEN_2=")) {
						hasNextPage = true
						return
					}
				})
			}
		}
	}

	// 3. Ищем любые элементы, которые могут быть номерами страниц
	if !hasNextPage {
		// Ищем все ссылки, которые могут быть пагинацией
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if exists && strings.Contains(href, "PAGEN_2=") {
				// Проверяем, есть ли ссылка на страницу с большим номером
				if strings.Contains(category.URL, "PAGEN_2=") {
					// Извлекаем текущий номер страницы из URL категории
					currentPageParts := strings.Split(category.URL, "PAGEN_2=")
					if len(currentPageParts) > 1 {
						currentPageStr := strings.Split(currentPageParts[1], "&")[0]
						currentPage, errCurr := strconv.Atoi(currentPageStr)

						// Извлекаем номер страницы из href
						nextPageParts := strings.Split(href, "PAGEN_2=")
						if len(nextPageParts) > 1 {
							nextPageStr := strings.Split(nextPageParts[1], "&")[0]
							nextPage, errNext := strconv.Atoi(nextPageStr)

							if errCurr == nil && errNext == nil && nextPage > currentPage {
								hasNextPage = true
								return
							}
						}
					}
				} else {
					// Если в текущем URL нет PAGEN_2, значит это первая страница
					hasNextPage = true
					return
				}
			}
		})
	}

	// 4. Анализируем HTML-код страницы на наличие скриптов с пагинацией
	if !hasNextPage {
		// Получаем весь HTML страницы
		html, err := doc.Html()
		if err == nil {
			// Ищем специфичные для Bitrix скрипты пагинации
			if strings.Contains(html, "NavPageNomer") && strings.Contains(html, "NavPageCount") {
				// Проверяем, совпадает ли текущая страница с последней
				if !strings.Contains(html, "NavPageNomer=NavPageCount") {
					hasNextPage = true
				}
			}
		}
	}

	// 5. Проверяем, есть ли на странице параметры для ajax-пагинации
	if !hasNextPage {
		doc.Find("script").Each(func(i int, s *goquery.Selection) {
			script := s.Text()
			if strings.Contains(script, "bxajaxid") && strings.Contains(script, "pagen") {
				hasNextPage = true
				return
			}
		})
	}

	log.Printf("На странице найдено %d товаров, есть следующая страница: %v", len(products), hasNextPage)

	return products, hasNextPage
}

// getProductDetails получает детальную информацию о товаре
func getProductDetails(url string, semaphore chan struct{}, delayMs int) (Product, error) {
	semaphore <- struct{}{}        // Занимаем слот в семафоре
	defer func() { <-semaphore }() // Освобождаем слот при выходе

	time.Sleep(time.Duration(delayMs) * time.Millisecond) // Задержка между запросами

	resp, err := doRequestWithRetry(url, 2, delayMs)
	if err != nil {
		return Product{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Product{}, fmt.Errorf("ошибка при получении страницы товара: %d", resp.StatusCode)
	}

	// Определяем кодировку и создаем Reader с преобразованием в UTF-8
	utf8Reader, err := getUTF8Reader(resp.Body)
	if err != nil {
		return Product{}, err
	}

	doc, err := goquery.NewDocumentFromReader(utf8Reader)
	if err != nil {
		return Product{}, err
	}

	var product Product

	// Извлекаем ID товара из URL или со страницы
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		product.ID = parts[len(parts)-2] // Предпоследний элемент в URL обычно ID товара
	}

	// Извлекаем описание товара
	description := strings.TrimSpace(doc.Find(".product__description").Text())
	if description == "" {
		description = strings.TrimSpace(doc.Find(".product-description").Text())
	}
	if description == "" {
		description = strings.TrimSpace(doc.Find(".description").Text())
	}
	product.Description = description

	// Извлекаем характеристики товара
	doc.Find(".product__specs tr, .product-features li, .specifications li").Each(func(i int, s *goquery.Selection) {
		feature := strings.TrimSpace(s.Text())
		if feature != "" {
			product.Features = append(product.Features, feature)
		}
	})

	return product, nil
}

// getUTF8Reader создает Reader с преобразованием в UTF-8
func getUTF8Reader(r io.Reader) (io.Reader, error) {
	// Пробуем автоматически определить кодировку
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Пробуем определить кодировку автоматически
	e, _, _ := charset.DetermineEncoding(b, "")

	// Если не удалось определить или определена неверно, пробуем Windows-1251 (распространенная для русских сайтов)
	contentStr := string(b)
	if strings.Contains(contentStr, "\xef\xbf\xbd") || strings.Contains(contentStr, "\ufffd") {
		e = charmap.Windows1251
	}

	// Создаем Reader с преобразованием в UTF-8
	return transform.NewReader(strings.NewReader(string(b)), e.NewDecoder()), nil
}

// saveToJSON сохраняет данные в JSON файл
func saveToJSON(data interface{}, filename string) error {
	// Создаем файл для записи с BOM
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Записываем BOM для корректного отображения UTF-8 в Windows
	bom := []byte{0xEF, 0xBB, 0xBF}
	if _, err := file.Write(bom); err != nil {
		return err
	}

	// Используем Encoder для экономии памяти при сериализации больших объемов данных
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")  // Устанавливаем отступы для читаемости
	encoder.SetEscapeHTML(false) // Не экранировать HTML-символы

	// Сериализуем данные непосредственно в файл
	if err := encoder.Encode(data); err != nil {
		return err
	}

	return nil
}

// saveToCSV сохраняет данные в CSV файл с разделителем ";"
func saveToCSV(products []Product, filename string) error {
	// Создаем файл с BOM для корректного отображения UTF-8 в Windows
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Записываем BOM
	bom := []byte{0xEF, 0xBB, 0xBF}
	if _, err := file.Write(bom); err != nil {
		return err
	}

	writer := csv.NewWriter(file)
	writer.Comma = ';' // Устанавливаем разделитель ";"

	// Увеличиваем буфер для CSV Writer для улучшения производительности
	// при большом количестве записей
	writer.UseCRLF = true // Использовать CRLF для совместимости с Windows
	defer writer.Flush()

	// Записываем заголовки
	headers := []string{"ID", "Название", "URL", "Описание", "Цена", "URL изображения", "Категория", "Характеристики"}
	if err := writer.Write(headers); err != nil {
		return err
	}

	// Пакетная запись для улучшения производительности
	batchSize := 1000
	records := make([][]string, 0, batchSize)

	// Записываем данные продуктов
	for _, product := range products {
		// Объединяем характеристики в одну строку, разделенную символом |
		featuresStr := strings.Join(product.Features, "|")

		record := []string{
			product.ID,
			product.Name,
			product.URL,
			product.Description,
			product.Price,
			product.ImageURL,
			product.Category,
			featuresStr,
		}

		records = append(records, record)

		// Когда накопилось достаточно записей, записываем их и сбрасываем массив
		if len(records) >= batchSize {
			if err := writer.WriteAll(records); err != nil {
				return err
			}
			records = records[:0]
		}
	}

	// Записываем оставшиеся записи
	if len(records) > 0 {
		if err := writer.WriteAll(records); err != nil {
			return err
		}
	}

	return nil
}

// enrichProductsWithDetails обогащает товары детальной информацией
func enrichProductsWithDetails(products []Product, semaphore chan struct{}, delayMs int) {
	// Создаем WaitGroup для ожидания завершения всех обогащений
	var wg sync.WaitGroup

	// Создаем канал для обновленных товаров
	productChan := make(chan Product, len(products))

	// Создаем переменные для отслеживания прогресса
	var processed, skipped, enriched, errors int
	var mutex sync.Mutex             // Мьютекс для безопасного обновления счетчиков
	errorMap := make(map[string]int) // Храним ошибки и их количество

	startTime := time.Now()

	// Функция для обновления и вывода прогресса
	updateProgress := func(action string, errorMsg string) {
		mutex.Lock()
		defer mutex.Unlock()

		switch action {
		case "processed":
			processed++
		case "skipped":
			skipped++
		case "enriched":
			enriched++
		case "error":
			errors++
			errorMap[errorMsg]++
		}

		// Каждые 10 товаров или по завершении выводим прогресс
		if processed%10 == 0 || processed == len(products) {
			progress := float64(processed) / float64(len(products)) * 100
			elapsed := time.Since(startTime)
			itemsPerSecond := float64(processed) / elapsed.Seconds()

			// Оценка оставшегося времени
			var eta time.Duration
			if processed > 0 {
				eta = time.Duration(float64(len(products)-processed) / itemsPerSecond * float64(time.Second))
			}

			log.Printf("Прогресс обогащения: %.1f%% (%d/%d) - Обогащено: %d, Пропущено: %d, Ошибок: %d, Скорость: %.1f товаров/сек, Осталось: %v",
				progress, processed, len(products), enriched, skipped, errors, itemsPerSecond, eta.Round(time.Second))
		}
	}

	log.Printf("Начинаем обогащение %d товаров детальной информацией...", len(products))

	// Вычисляем размер батча для вывода прогресса - используется в updateProgress
	batchSize := maxNum(1, len(products)/20) // 5% шаг

	// Обновляем логику обновления прогресса с использованием batchSize
	oldUpdateProgress := updateProgress
	updateProgress = func(action string, errorMsg string) {
		oldUpdateProgress(action, errorMsg)
		// Выводим прогресс каждые batchSize товаров вместо каждых 10
		if processed%batchSize == 0 || processed == len(products) {
			progress := float64(processed) / float64(len(products)) * 100
			elapsed := time.Since(startTime)
			itemsPerSecond := float64(processed) / elapsed.Seconds()

			// Оценка оставшегося времени
			var eta time.Duration
			if processed > 0 {
				eta = time.Duration(float64(len(products)-processed) / itemsPerSecond * float64(time.Second))
			}

			log.Printf("Прогресс обогащения: %.1f%% (%d/%d) - Обогащено: %d, Пропущено: %d, Ошибок: %d, Скорость: %.1f товаров/сек, Осталось: %v",
				progress, processed, len(products), enriched, skipped, errors, itemsPerSecond, eta.Round(time.Second))
		}
	}

	// Обогащаем каждый товар в отдельной горутине
	for i := range products {
		// Если у товара уже есть характеристики, пропускаем его
		if len(products[i].Features) > 0 && products[i].Description != "" {
			productChan <- products[i]
			updateProgress("skipped", "")
			continue
		}

		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			prod := products[index]

			// Получаем детальную информацию о товаре
			details, err := getProductDetails(prod.URL, semaphore, delayMs)
			if err != nil {
				errorMsg := fmt.Sprintf("%v", err)
				log.Printf("Ошибка при получении деталей товара ID=%s, URL=%s: %v",
					prod.ID, prod.URL, err)
				productChan <- prod
				updateProgress("error", errorMsg)
				return
			}

			// Обновляем описание и характеристики, если они не пустые
			if details.Description != "" {
				prod.Description = details.Description
			}

			if len(details.Features) > 0 {
				prod.Features = details.Features
			}

			productChan <- prod
			updateProgress("enriched", "")
		}(i)

		updateProgress("processed", "")
	}

	// Горутина для закрытия канала после завершения всех обработок
	go func() {
		wg.Wait()
		close(productChan)
	}()

	// Собираем обогащенные товары
	enrichedProducts := make([]Product, 0, len(products))
	for product := range productChan {
		enrichedProducts = append(enrichedProducts, product)
	}

	// Очищаем исходный слайс и копируем в него обогащенные товары
	// Это безопасно работает даже если количество товаров изменилось
	// из-за дедупликации
	if len(enrichedProducts) > 0 {
		// Очищаем products, сохраняя его ёмкость
		products = products[:0]

		// Добавляем обогащенные товары
		products = append(products, enrichedProducts...)
	}

	totalTime := time.Since(startTime)
	itemsPerSecond := float64(len(products)) / totalTime.Seconds()

	log.Printf("Обогащение завершено: Всего товаров: %d, Обогащено: %d, Пропущено: %d, Ошибок: %d, Время: %v, Средняя скорость: %.1f товаров/сек",
		len(products), enriched, skipped, errors, totalTime.Round(time.Second), itemsPerSecond)

	// Выводим статистику по ошибкам
	if errors > 0 {
		log.Println("Статистика ошибок:")
		for errMsg, count := range errorMap {
			log.Printf("  - %s: %d раз", errMsg, count)
		}
	}
}

// inspectPaginationOnCategory исследует пагинацию на странице категории
func inspectPaginationOnCategory(url string) {
	fmt.Printf("Исследование пагинации для URL: %s\n", url)

	resp, err := doRequestWithRetry(url, 3, delay)
	if err != nil {
		log.Fatalf("Ошибка при получении страницы: %v", err)
	}
	defer resp.Body.Close()

	// Определяем кодировку и создаем Reader с преобразованием в UTF-8
	utf8Reader, err := getUTF8Reader(resp.Body)
	if err != nil {
		log.Fatalf("Ошибка при определении кодировки: %v", err)
	}

	doc, err := goquery.NewDocumentFromReader(utf8Reader)
	if err != nil {
		log.Fatalf("Ошибка при парсинге HTML: %v", err)
	}

	// Создаем файл для вывода результатов
	f, err := os.Create("pagination_structure.txt")
	if err != nil {
		log.Fatalf("Ошибка при создании файла: %v", err)
	}
	defer f.Close()

	// Выводим информацию о странице
	fmt.Fprintln(f, "=== ИССЛЕДОВАНИЕ ПАГИНАЦИИ ===")
	fmt.Fprintf(f, "URL: %s\n\n", url)

	// Ищем все элементы, которые могут быть пагинацией
	paginationSelectors := []string{
		".pagination", ".paginations", ".nav-links", ".pager",
		".pages", ".pagenation", ".modern-page-navigation",
	}

	fmt.Fprintln(f, "=== ЭЛЕМЕНТЫ ПАГИНАЦИИ ===")
	for _, selector := range paginationSelectors {
		elements := doc.Find(selector)
		fmt.Fprintf(f, "Селектор: %s\n", selector)
		fmt.Fprintf(f, "Найдено элементов: %d\n", elements.Length())

		if elements.Length() > 0 {
			html, _ := elements.Html()
			fmt.Fprintf(f, "HTML:\n%s\n", html)

			// Ищем ссылки на страницы
			elements.Find("a").Each(func(i int, s *goquery.Selection) {
				href, exists := s.Attr("href")
				if exists {
					fmt.Fprintf(f, "Ссылка #%d: %s -> %s\n", i+1, strings.TrimSpace(s.Text()), href)
				}
			})
		}
		fmt.Fprintln(f, "---")
	}

	// Ищем все ссылки, содержащие PAGEN_2
	fmt.Fprintln(f, "\n=== ССЫЛКИ С PAGEN_2 ===")
	doc.Find("a[href*='PAGEN_2']").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		text := strings.TrimSpace(s.Text())
		fmt.Fprintf(f, "Ссылка #%d: %s -> %s\n", i+1, text, href)
	})

	// Ищем все скрипты, которые могут содержать информацию о пагинации
	fmt.Fprintln(f, "\n=== СКРИПТЫ С ИНФОРМАЦИЕЙ О ПАГИНАЦИИ ===")
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		script := s.Text()
		if strings.Contains(script, "NavPageNomer") ||
			strings.Contains(script, "NavPageCount") ||
			strings.Contains(script, "bxajaxid") ||
			strings.Contains(script, "pagen") {
			fmt.Fprintf(f, "Скрипт #%d:\n%s\n---\n", i+1, script)
		}
	})

	// Симулируем функцию extractProductsFromPage для проверки работы определения наличия следующей страницы
	products, hasNextPage := extractProductsFromPage(doc, Category{URL: url, Name: "Test"})

	fmt.Fprintf(f, "\n=== РЕЗУЛЬТАТЫ АНАЛИЗА ===\n")
	fmt.Fprintf(f, "Найдено товаров: %d\n", len(products))
	fmt.Fprintf(f, "Есть следующая страница: %v\n", hasNextPage)

	fmt.Printf("Исследование завершено. Результаты сохранены в файл pagination_structure.txt\n")
}

// removeDuplicateProducts удаляет дубликаты товаров из массива по ID
func removeDuplicateProducts(products []Product) []Product {
	// Создаем карту для хранения уникальных товаров
	uniqueMap := make(map[string]Product)

	// Создаем отображение для подсчета дубликатов
	duplicateCount := make(map[string]int)

	// Заполняем карту, используя ID товара как ключ
	for _, product := range products {
		if product.ID == "" {
			continue // Пропускаем товары без ID
		}

		uniqueMap[product.ID] = product
		duplicateCount[product.ID]++
	}

	// Выводим информацию о найденных дубликатах
	duplicatesFound := 0
	maxDuplicates := 0
	var maxDuplicateID string

	for id, count := range duplicateCount {
		if count > 1 {
			duplicatesFound++
			if count > maxDuplicates {
				maxDuplicates = count
				maxDuplicateID = id
			}
		}
	}

	if duplicatesFound > 0 {
		fmt.Printf("Найдено %d товаров с дубликатами. Максимальное количество дубликатов: %d для товара ID %s\n",
			duplicatesFound, maxDuplicates, maxDuplicateID)
	}

	// Создаем новый массив с уникальными товарами
	uniqueProducts := make([]Product, 0, len(uniqueMap))
	for _, product := range uniqueMap {
		uniqueProducts = append(uniqueProducts, product)
	}

	return uniqueProducts
}

// Max возвращает максимальное из двух целых чисел
func maxNum(a, b int) int {
	if a > b {
		return a
	}
	return b
}
