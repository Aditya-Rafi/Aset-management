// Package seed loads the starting data: default categories and the admin account.
package seed

import (
	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

var computer = []string{"cpu", "ram", "storage", "os"}

// DefaultCategories cover what an IT support desk usually tracks. Each tag
// prefix (code) is what ends up on the sticker, e.g. LPT-0001.
var DefaultCategories = []models.Category{
	{Name: "Laptop", Code: "LPT", SpecFields: computer, Inspection: "laptop"},
	{Name: "PC", Code: "PC", SpecFields: computer, Inspection: "general"},
	{Name: "Monitor", Code: "MON", SpecFields: []string{"screen_size"}, Inspection: "general"},
	{Name: "Handphone", Code: "HP", SpecFields: []string{"ram", "storage", "os"}, Inspection: "general"},
	{Name: "Kartu SIM", Code: "SIM"},
	{Name: "Printer", Code: "PRT", Inspection: "general"},
	{Name: "Kamera", Code: "CAM", Inspection: "general"},
	{Name: "Mouse", Code: "MSE"},
	{Name: "Keyboard", Code: "KBD"},
	{Name: "Tas Laptop", Code: "BAG"},
	{Name: "Converter / Adaptor", Code: "CNV"},
	{Name: "PSU / UPS", Code: "PSU"},
	{Name: "Power Stabilizer", Code: "STB"},
	{Name: "Antena Wireless", Code: "ANT"},
	{Name: "RAM", Code: "RAM", SpecFields: []string{"ram"}},
	{Name: "HDD", Code: "HDD", SpecFields: []string{"storage"}},
	{Name: "SSD", Code: "SSD", SpecFields: []string{"storage"}},
	{Name: "Kabel", Code: "CBL"},
	{Name: "Lainnya", Code: "OTH"},
}

// Run fills an empty database with the default categories. The first admin is
// created separately by auth.Bootstrap. Existing data is never touched.
func Run(db *gorm.DB) error {
	var n int64
	if err := db.Model(&models.Category{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	cats := make([]models.Category, len(DefaultCategories))
	copy(cats, DefaultCategories)
	return db.Create(&cats).Error
}
