package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Эта программа используется для изучения HTML-структуры сайта
// и настройки селекторов для парсера

func inspectMain() {
	// Исследуем структуру каталога
	err := inspectCatalogPage()
	if err != nil {
		log.Fatalf("Ошибка при исследовании каталога: %v", err)
	}

	fmt.Println("Исследование каталога завершено. Результаты сохранены в catalog_structure.txt")

	// Исследуем страницу категории
	err = inspectCategoryPage("https://www.stanki.ru/catalog/metalloobrabatyvayuschee_oborudovanie/")
	if err != nil {
		log.Fatalf("Ошибка при исследовании категории: %v", err)
	}

	fmt.Println("Исследование категории завершено. Результаты сохранены в category_structure.txt")
}

// inspectCatalogPage исследует структуру главной страницы каталога
func inspectCatalogPage() error {
	resp, err := http.Get("https://www.stanki.ru/catalog/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ошибка при получении страницы каталога: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return err
	}

	// Создаем файл для вывода результатов
	f, err := os.Create("catalog_structure.txt")
	if err != nil {
		return err
	}
	defer f.Close()

	// Ищем все возможные категории и их селекторы
	fmt.Fprintln(f, "=== СТРУКТУРА КАТАЛОГА ===")

	// Проверяем заголовок страницы
	title := doc.Find("title").Text()
	fmt.Fprintf(f, "Заголовок страницы: %s\n\n", title)

	// Исследуем различные более конкретные селекторы
	selectors := []string{
		"ul.catalog", "ul.catalog li", "div.catalog", "div.catalog a",
		".catalog__list", ".catalog-list", ".category-list", ".catalog-categories",
		".content a", ".content li", ".catalog-wrapper a", ".catalog-item",
		"#catalog", "#catalog-list", ".left-menu a", ".sidebar a",
		"div.catalog-section", "div.catalog-section a",
	}

	for _, selector := range selectors {
		elements := doc.Find(selector)
		count := elements.Length()
		fmt.Fprintf(f, "Селектор: %s\n", selector)
		fmt.Fprintf(f, "Найдено элементов: %d\n", count)

		if count > 0 {
			// Выводим первые 5 найденных элементов для анализа
			elements.Each(func(i int, s *goquery.Selection) {
				if i < 5 {
					text := strings.TrimSpace(s.Text())
					fmt.Fprintf(f, "Элемент #%d (текст): %s\n", i+1, text)

					// Проверяем наличие ссылок
					s.Find("a").Each(func(j int, a *goquery.Selection) {
						href, exists := a.Attr("href")
						if exists {
							linkText := strings.TrimSpace(a.Text())
							fmt.Fprintf(f, "  Ссылка: %s -> %s\n", linkText, href)
						}
					})

					// Если сам элемент является ссылкой
					if s.Is("a") {
						href, exists := s.Attr("href")
						if exists {
							fmt.Fprintf(f, "  Это ссылка: %s\n", href)
						}
					}

					fmt.Fprintln(f, "---")
				}
			})
		}
		fmt.Fprintln(f, "===")
	}

	// Дополнительно исследуем страницу на наличие блоков с каталогом
	fmt.Fprintln(f, "=== ПОИСК БЛОКОВ С КАТАЛОГОМ ===")

	// Проверяем все div с классами, содержащими слово "catalog"
	doc.Find("div[class*='catalog']").Each(func(i int, s *goquery.Selection) {
		if i < 10 { // Ограничимся первыми 10 блоками
			class, _ := s.Attr("class")
			fmt.Fprintf(f, "Блок #%d, класс: %s\n", i+1, class)

			// Ищем ссылки внутри блока
			links := s.Find("a")
			fmt.Fprintf(f, "  Ссылок внутри: %d\n", links.Length())

			links.Each(func(j int, a *goquery.Selection) {
				if j < 5 { // Показываем только первые 5 ссылок
					href, exists := a.Attr("href")
					if exists {
						linkText := strings.TrimSpace(a.Text())
						fmt.Fprintf(f, "    Ссылка %d: %s -> %s\n", j+1, linkText, href)
					}
				}
			})

			fmt.Fprintln(f, "---")
		}
	})

	// Проверяем все div с id, содержащими слово "catalog"
	doc.Find("div[id*='catalog']").Each(func(i int, s *goquery.Selection) {
		id, _ := s.Attr("id")
		fmt.Fprintf(f, "Блок с id: %s\n", id)

		// Ищем ссылки внутри блока
		links := s.Find("a")
		fmt.Fprintf(f, "  Ссылок внутри: %d\n", links.Length())

		links.Each(func(j int, a *goquery.Selection) {
			if j < 5 { // Показываем только первые 5 ссылок
				href, exists := a.Attr("href")
				if exists {
					linkText := strings.TrimSpace(a.Text())
					fmt.Fprintf(f, "    Ссылка %d: %s -> %s\n", j+1, linkText, href)
				}
			}
		})

		fmt.Fprintln(f, "---")
	})

	// Проверяем все ссылки, начинающиеся с /catalog/
	fmt.Fprintln(f, "\n=== ССЫЛКИ НА КАТЕГОРИИ ===")
	doc.Find("a[href^='/catalog/']").Each(func(i int, a *goquery.Selection) {
		if i < 20 { // Ограничимся первыми 20 ссылками
			href, _ := a.Attr("href")
			text := strings.TrimSpace(a.Text())
			fmt.Fprintf(f, "Ссылка #%d: %s -> %s\n", i+1, text, href)
		}
	})

	return nil
}

// inspectCategoryPage исследует структуру страницы категории
func inspectCategoryPage(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ошибка при получении страницы категории: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return err
	}

	// Создаем файл для вывода результатов
	f, err := os.Create("category_structure.txt")
	if err != nil {
		return err
	}
	defer f.Close()

	// Заголовок страницы
	title := doc.Find("title").Text()
	fmt.Fprintf(f, "=== СТРУКТУРА СТРАНИЦЫ КАТЕГОРИИ ===\n")
	fmt.Fprintf(f, "URL: %s\n", url)
	fmt.Fprintf(f, "Заголовок: %s\n\n", title)

	// Расширенный анализ страницы

	// 1. Проверяем наличие подкатегорий
	fmt.Fprintln(f, "=== ПОДКАТЕГОРИИ ===")
	subCategorySelectors := []string{
		"a[href^='/catalog/']", ".subcategory", ".category-item", ".subcategory-list a",
		".category-list a", ".catalog__subcategory", ".catalog a",
	}

	for _, selector := range subCategorySelectors {
		elements := doc.Find(selector)
		fmt.Fprintf(f, "Селектор: %s\n", selector)
		fmt.Fprintf(f, "Найдено элементов: %d\n", elements.Length())

		if elements.Length() > 0 {
			elements.Each(func(i int, s *goquery.Selection) {
				if i < 10 {
					href, exists := s.Attr("href")
					text := strings.TrimSpace(s.Text())

					if exists && strings.HasPrefix(href, "/catalog/") && text != "" {
						fmt.Fprintf(f, "  Подкатегория #%d: %s -> %s\n", i+1, text, href)
					}
				}
			})
		}
		fmt.Fprintln(f, "---")
	}

	// 2. Поиск элементов товаров
	fmt.Fprintln(f, "\n=== ТОВАРЫ ===")

	// Расширенный список возможных селекторов товаров
	productSelectors := []string{
		".catalog-cards .catalog-card", ".catalog-item", ".product", ".product-item",
		".item", ".goods-item", ".product-card", ".product-list-item",
		"[itemtype='http://schema.org/Product']", ".catalog__product",
		"[data-product-id]", ".catalog-grid__item", ".catalog-element",
		".card", ".product-box", ".goods", ".list-item",
		"div[class*='product']", "div[class*='catalog'] div[class*='item']",
		".catalog__main .catalog-card",
	}

	for _, selector := range productSelectors {
		elements := doc.Find(selector)
		fmt.Fprintf(f, "Селектор товаров: %s\n", selector)
		fmt.Fprintf(f, "Найдено элементов: %d\n", elements.Length())

		if elements.Length() > 0 {
			elements.Each(func(i int, s *goquery.Selection) {
				if i < 3 { // Показываем только первые 3 товара
					html, _ := s.Html()
					fmt.Fprintf(f, "Товар #%d HTML:\n%s\n", i+1, html)

					// Ищем название товара
					nameSelectors := []string{"h2", "h3", "h4", ".name", ".title", ".product-name", "a"}
					for _, nameSelector := range nameSelectors {
						name := strings.TrimSpace(s.Find(nameSelector).First().Text())
						if name != "" {
							fmt.Fprintf(f, "Название (%s): %s\n", nameSelector, name)
							break
						}
					}

					// Ищем ссылку на товар
					s.Find("a").Each(func(j int, a *goquery.Selection) {
						if j < 2 { // Первые две ссылки
							href, exists := a.Attr("href")
							if exists {
								fmt.Fprintf(f, "Ссылка: %s -> %s\n", strings.TrimSpace(a.Text()), href)
							}
						}
					})

					fmt.Fprintln(f, "---")
				}
			})
		}
		fmt.Fprintln(f, "===")
	}

	// 3. Проверяем все ссылки на странице
	fmt.Fprintln(f, "\n=== АНАЛИЗ ССЫЛОК ===")

	// Проверяем все ссылки, которые могут быть на товары
	var productLinks []string
	doc.Find("a").Each(func(i int, a *goquery.Selection) {
		href, exists := a.Attr("href")
		if exists && strings.Contains(href, "/catalog/") {
			// Проверяем расширения, которые могут указывать на страницу товара
			if strings.HasSuffix(href, ".html") || !strings.Contains(href, ".") {
				productLinks = append(productLinks, href)
			}
		}
	})

	// Выводим уникальные ссылки на товары
	uniqueLinks := make(map[string]bool)
	for _, link := range productLinks {
		uniqueLinks[link] = true
	}

	fmt.Fprintf(f, "Найдено %d уникальных ссылок на возможные товары\n", len(uniqueLinks))
	i := 0
	for link := range uniqueLinks {
		if i < 10 { // Выводим только первые 10 ссылок
			fmt.Fprintf(f, "  Ссылка #%d: %s\n", i+1, link)
		}
		i++
	}

	// 4. Поиск блоков с товарами по классам, содержащим характерные слова
	fmt.Fprintln(f, "\n=== ПОИСК БЛОКОВ С ТОВАРАМИ ===")

	blockSelectors := []string{
		"div[class*='catalog']", "div[class*='product']", "div[class*='item']",
		"div[class*='card']", "div[class*='goods']", "div[class*='list']",
	}

	for _, selector := range blockSelectors {
		elements := doc.Find(selector)
		fmt.Fprintf(f, "Селектор: %s\n", selector)
		fmt.Fprintf(f, "Найдено элементов: %d\n", elements.Length())

		if elements.Length() > 0 {
			fmt.Fprintln(f, "Классы найденных элементов:")
			elements.Each(func(i int, s *goquery.Selection) {
				if i < 10 {
					class, _ := s.Attr("class")
					fmt.Fprintf(f, "  #%d: %s\n", i+1, class)

					// Проверяем наличие ссылок внутри блока
					links := s.Find("a")
					if links.Length() > 0 {
						fmt.Fprintf(f, "    Содержит %d ссылок\n", links.Length())

						links.Each(func(j int, a *goquery.Selection) {
							if j < 2 {
								href, exists := a.Attr("href")
								if exists {
									fmt.Fprintf(f, "    Ссылка: %s -> %s\n", strings.TrimSpace(a.Text()), href)
								}
							}
						})
					}
				}
			})
		}
		fmt.Fprintln(f, "---")
	}

	return nil
}

// inspectProductPage исследует структуру страницы товара
func inspectProductPage(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ошибка при получении страницы товара: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return err
	}

	// Создаем файл для вывода результатов
	f, err := os.Create("product_structure.txt")
	if err != nil {
		return err
	}
	defer f.Close()

	// Ищем структуру описания товара
	fmt.Fprintln(f, "=== СТРУКТУРА СТРАНИЦЫ ТОВАРА ===")

	// Пытаемся найти название
	nameSelectors := []string{"h1", ".product-name", ".product-title", ".page-title"}
	for _, selector := range nameSelectors {
		name := strings.TrimSpace(doc.Find(selector).First().Text())
		if name != "" {
			fmt.Fprintf(f, "Название товара (%s): %s\n", selector, name)
		}
	}

	// Пытаемся найти цену
	priceSelectors := []string{".price", ".product-price", ".price-value", ".cost"}
	for _, selector := range priceSelectors {
		price := strings.TrimSpace(doc.Find(selector).First().Text())
		if price != "" {
			fmt.Fprintf(f, "Цена (%s): %s\n", selector, price)
		}
	}

	// Пытаемся найти описание
	descSelectors := []string{".description", ".product-description", ".details", ".product-details"}
	for _, selector := range descSelectors {
		desc := strings.TrimSpace(doc.Find(selector).First().Text())
		if desc != "" {
			if len(desc) > 200 {
				desc = desc[:200] + "..." // Ограничиваем вывод для удобства чтения
			}
			fmt.Fprintf(f, "Описание (%s): %s\n", selector, desc)
		}
	}

	// Пытаемся найти характеристики
	featureSelectors := []string{".features", ".specifications", ".product-features", ".characteristics", "table.specs"}
	for _, selector := range featureSelectors {
		fmt.Fprintf(f, "Характеристики (%s):\n", selector)
		doc.Find(selector).Each(func(i int, s *goquery.Selection) {
			html, _ := s.Html()
			fmt.Fprintf(f, "HTML блока характеристик: %s\n", html)

			// Проверяем дочерние элементы для поиска конкретных характеристик
			s.Find("li, tr").Each(func(j int, feature *goquery.Selection) {
				if j < 5 { // Выводим не более 5 характеристик для примера
					fmt.Fprintf(f, "  Характеристика #%d: %s\n", j+1, strings.TrimSpace(feature.Text()))
				}
			})
		})
	}

	// Пытаемся найти изображения
	imgSelectors := []string{".product-image", ".gallery", ".product-gallery", ".images"}
	for _, selector := range imgSelectors {
		fmt.Fprintf(f, "Изображения (%s):\n", selector)
		doc.Find(selector + " img").Each(func(i int, img *goquery.Selection) {
			src, exists := img.Attr("src")
			if exists {
				fmt.Fprintf(f, "  Изображение #%d: %s\n", i+1, src)
			}
		})
	}

	return nil
}
