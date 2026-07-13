package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/platform/config"
	"kita-be/internal/platform/database"
	"kita-be/internal/platform/logger"
)

var sampleBooks = []struct {
	ISBN        string
	Title       string
	Author      string
	Publisher   string
	Category    string
	Description string
	TotalStock  int
}{
	{
		ISBN: "978-602-03-3258-8", Title: "Laskar Pelangi", Author: "Andrea Hirata",
		Publisher: "Bentang Pustaka", Category: "Fiksi",
		Description: "Novel inspiratif tentang perjuangan anak-anak di Belitung.", TotalStock: 5,
	},
	{
		ISBN: "978-979-22-3326-0", Title: "Bumi Manusia", Author: "Pramoedya Ananta Toer",
		Publisher: "Hasta Mitra", Category: "Fiksi",
		Description: "Novel sejarah tentang perjuangan rakyat Indonesia melawan kolonialisme.", TotalStock: 3,
	},
	{
		ISBN: "978-602-291-336-7", Title: "Filosofi Teras", Author: "Henry Manampiring",
		Publisher: "Kompas", Category: "Non-Fiksi",
		Description: "Buku tentang stoicisme dan penerapannya dalam kehidupan sehari-hari.", TotalStock: 7,
	},
	{
		ISBN: "978-623-00-1156-7", Title: "Atomic Habits", Author: "James Clear",
		Publisher: "Gramedia Pustaka Utama", Category: "Non-Fiksi",
		Description: "Cara mudah mengubah kebiasaan kecil untuk hasil yang besar.", TotalStock: 4,
	},
	{
		ISBN: "978-602-06-0841-0", Title: "Supernova: Ksatria, Puteri, dan Bintang Jatuh", Author: "Dee Lestari",
		Publisher: "Truedee Books", Category: "Fiksi",
		Description: "Novel fiksi ilmiah yang menggabungkan sains dan spiritualitas.", TotalStock: 2,
	},
	{
		ISBN: "978-979-794-537-4", Title: "Laut Bercerita", Author: "Leila S. Chudori",
		Publisher: "KPG", Category: "Fiksi",
		Description: "Novel tentang aktivis yang hilang pada masa reformasi 1998.", TotalStock: 6,
	},
	{
		ISBN: "978-602-424-694-4", Title: "Sapiens: A Brief History of Humankind", Author: "Yuval Noah Harari",
		Publisher: "KPG", Category: "Non-Fiksi",
		Description: "Sejarah singkat umat manusia dari zaman purba hingga modern.", TotalStock: 3,
	},
	{
		ISBN: "978-602-03-7719-7", Title: "Cantik Itu Luka", Author: "Eka Kurniawan",
		Publisher: "Gramedia Pustaka Utama", Category: "Fiksi",
		Description: "Novel epik tentang keluarga di masa penjajahan dan kemerdekaan.", TotalStock: 4,
	},
	{
		ISBN: "978-623-00-1522-0", Title: "The Lean Startup", Author: "Eric Ries",
		Publisher: "Bentang Pustaka", Category: "Non-Fiksi",
		Description: "Panduan membangun startup dengan pendekatan lean.", TotalStock: 5,
	},
	{
		ISBN: "978-602-03-1392-4", Title: "Pulang", Author: "Tere Liye",
		Publisher: "Republika", Category: "Fiksi",
		Description: "Novel petualangan tentang seorang anak yang mencari jati diri.", TotalStock: 0,
	},
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	db, err := database.NewPool(cfg)
	if err != nil {
		logger.Error("failed to connect to database", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	ctx := context.Background()

	for _, b := range sampleBooks {
		book := domain.NewBook(uuid.New().String(), b.ISBN, b.Title, b.Author, b.TotalStock)
		pub := b.Publisher
		book.Publisher = &pub
		cat := b.Category
		book.Category = &cat
		desc := b.Description
		book.Description = &desc

		_, err := db.Exec(ctx,
			`INSERT INTO books (id, isbn, title, author, publisher, category, description, total_stock, available_stock, status, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			 ON CONFLICT (isbn) DO NOTHING`,
			book.ID, book.ISBN, book.Title, book.Author, book.Publisher, book.Category,
			book.Description, book.TotalStock, book.AvailableStock, string(book.Status),
			book.CreatedAt, book.UpdatedAt,
		)
		if err != nil {
			logger.Error("failed to seed book", "isbn", b.ISBN, "error", err.Error())
			continue
		}
		fmt.Printf("Seeded: %s by %s (stock: %d)\n", b.Title, b.Author, b.TotalStock)
	}

	fmt.Println("Seed completed.")
}
