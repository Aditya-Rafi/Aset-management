// Package models defines the database schema of the IT asset inventory.
package models

import (
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Asset lifecycle status.
const (
	StatusAvailable = "available" // ready to be handed out (Tersedia / bisa dipakai)
	StatusInUse     = "in_use"    // handed out to someone (Dipakai)
	StatusRepair    = "repair"    // being serviced or under warranty claim (Diservis)
	StatusBroken    = "broken"    // not usable (Rusak)
	StatusDisposed  = "disposed"  // written off (Dimusnahkan)
)

// Handover document types.
const (
	TxOut      = "out"      // serah terima: asset handed to an employee
	TxIn       = "in"       // pengembalian: asset returned by an employee
	TxTransfer = "transfer" // pindah tangan: asset passes from one employee to another
)

// Asset log actions.
const (
	LogCreated     = "created"
	LogUpdated     = "updated"
	LogStatus      = "status"
	LogAssigned    = "assigned"
	LogReturned    = "returned"
	LogNote        = "note"
	LogPhoto       = "photo"
	LogReverted    = "reverted"
	LogTransferred = "transferred"
)

var AllStatuses = []string{StatusAvailable, StatusInUse, StatusRepair, StatusBroken, StatusDisposed}

func ValidStatus(s string) bool {
	for _, v := range AllStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// SpecFields are the optional hardware attributes a category can enable.
var SpecFields = []string{"cpu", "ram", "storage", "os", "screen_size"}

// Date is a calendar date serialised as YYYY-MM-DD.
type Date struct{ time.Time }

const dateLayout = "2006-01-02"

func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + d.Format(dateLayout) + `"`), nil
}

func (d *Date) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" || s == `""` {
		*d = Date{}
		return nil
	}
	if len(s) < 2 || s[0] != '"' {
		return fmt.Errorf("invalid date")
	}
	if len(s) > 12 { // tolerate full ISO timestamps
		s = s[:11] + `"`
	}
	t, err := time.Parse(`"`+dateLayout+`"`, s)
	if err != nil {
		return fmt.Errorf("invalid date, expected YYYY-MM-DD")
	}
	d.Time = t
	return nil
}

func (d Date) Value() (driver.Value, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.Format(dateLayout), nil
}

func (d *Date) Scan(v interface{}) error {
	switch x := v.(type) {
	case time.Time:
		d.Time = x
	case []byte:
		return d.parse(string(x))
	case string:
		return d.parse(x)
	case nil:
		*d = Date{}
	default:
		return fmt.Errorf("cannot scan %T into Date", v)
	}
	return nil
}

func (d *Date) parse(s string) error {
	if len(s) > 10 {
		s = s[:10]
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return err
	}
	d.Time = t
	return nil
}

type Employee struct {
	ID         uint    `gorm:"primaryKey" json:"id"`
	Name       string  `gorm:"size:255;not null;index" json:"name"`
	NIK        *string `gorm:"column:nik;size:64" json:"nik"` // employee id printed on the BAST
	Department *string `gorm:"size:255" json:"department"`
	Hp         *string `gorm:"size:255" json:"hp"`
	Email      *string `gorm:"size:255" json:"email"`
	Address    *string `gorm:"type:text" json:"address"`
	// Status follows the employee's life at the company: active people can
	// receive assets, inactive ones (resigned/ended) only keep their history.
	Status     string  `gorm:"size:10;not null;default:active;index" json:"status"`
	JoinedAt   *string `gorm:"size:10" json:"joined_at"` // YYYY-MM-DD
	LeftAt     *string `gorm:"size:10" json:"left_at"`   // YYYY-MM-DD, set when inactive
	ExitReason *string `gorm:"size:255" json:"exit_reason"`
	// AssetCount is filled by list queries only (assets held right now).
	AssetCount int64     `gorm:"->;-:migration" json:"asset_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

const (
	EmployeeActive   = "active"
	EmployeeInactive = "inactive"
)

// Category groups assets and decides which hardware fields the form shows.
type Category struct {
	ID         uint     `gorm:"primaryKey" json:"id"`
	Name       string   `gorm:"size:255;uniqueIndex;not null" json:"name"`
	Code       string   `gorm:"size:5;uniqueIndex;not null" json:"code"` // prefix of the asset tag, e.g. LPT
	Fields     string   `gorm:"column:spec_fields;size:255" json:"-"`
	SpecFields []string `gorm:"-" json:"spec_fields"`
	// Inspection selects the physical-check checklist used on handover documents: none, general or laptop.
	Inspection string    `gorm:"size:10;not null;default:none" json:"inspection"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (c *Category) AfterFind(*gorm.DB) error {
	c.SpecFields = []string{}
	for _, f := range strings.Split(c.Fields, ",") {
		if f = strings.TrimSpace(f); f != "" {
			c.SpecFields = append(c.SpecFields, f)
		}
	}
	return nil
}

// BeforeSave keeps only known field names, in canonical order.
func (c *Category) BeforeSave(*gorm.DB) error {
	var keep []string
	for _, known := range SpecFields {
		for _, f := range c.SpecFields {
			if f == known {
				keep = append(keep, known)
				break
			}
		}
	}
	c.SpecFields = keep
	c.Fields = strings.Join(keep, ",")
	return nil
}

var codeRe = regexp.MustCompile(`^[A-Z]{2,5}$`)

func ValidCode(code string) bool { return codeRe.MatchString(code) }

type Asset struct {
	ID              uint         `gorm:"primaryKey" json:"id"`
	Tag             string       `gorm:"size:32;uniqueIndex;not null" json:"tag"`
	CategoryID      uint         `gorm:"not null;index" json:"category_id"`
	Brand           *string      `gorm:"size:255" json:"brand"`
	Model           *string      `gorm:"size:255" json:"model"`
	SerialNumber    *string      `gorm:"size:255;index" json:"serial_number"`
	CPU             *string      `gorm:"column:cpu;size:255" json:"cpu"`
	RAM             *string      `gorm:"column:ram;size:100" json:"ram"`
	Storage         *string      `gorm:"size:255" json:"storage"`
	OS              *string      `gorm:"column:os;size:100" json:"os"`
	ScreenSize      *string      `gorm:"size:50" json:"screen_size"`
	ManufactureYear *int         `gorm:"type:smallint" json:"manufacture_year"`
	PurchaseDate    Date         `gorm:"type:date" json:"purchase_date"`
	PurchasePrice   *int64       `json:"purchase_price"`
	WarrantyUntil   Date         `gorm:"type:date;index" json:"warranty_until"`
	Vendor          *string      `gorm:"size:255" json:"vendor"`
	Status          string       `gorm:"size:20;not null;default:available;index" json:"status"`
	Location        *string      `gorm:"size:255" json:"location"`
	HolderID        *uint        `gorm:"index" json:"holder_id"`
	Quantity        int          `gorm:"type:int;not null;default:1" json:"quantity"`
	Notes           *string      `gorm:"type:text" json:"notes"`
	Category        *Category    `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"category,omitempty"`
	Holder          *Employee    `gorm:"foreignKey:HolderID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"holder,omitempty"`
	Images          []AssetImage `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"images"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

type AssetImage struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AssetID   uint      `gorm:"not null;index" json:"asset_id"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (AssetImage) TableName() string { return "asset_images" }

// Transaction is a handover (out) or return (in) document for one employee.
type Transaction struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	OrderNumber string `gorm:"size:40;uniqueIndex;not null" json:"order_number"`
	Type        string `gorm:"size:8;not null;index" json:"type"`
	EmployeeID  uint   `gorm:"not null;index" json:"employee_id"` // out: receiver, in: returner, transfer: receiver
	// FromEmployeeID is the previous holder on a transfer.
	FromEmployeeID *uint               `gorm:"index" json:"from_employee_id"`
	Note           *string             `gorm:"size:255" json:"note"`
	CreatedByID    *uint               `gorm:"index" json:"created_by_id"`
	Employee       *Employee           `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"employee,omitempty"`
	FromEmployee   *Employee           `gorm:"foreignKey:FromEmployeeID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"from_employee,omitempty"`
	Inspections    []Inspection        `gorm:"foreignKey:TransactionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"inspections,omitempty"`
	CreatedBy      *User               `gorm:"foreignKey:CreatedByID;constraint:OnDelete:SET NULL" json:"created_by,omitempty"`
	Details        []TransactionDetail `gorm:"foreignKey:TransactionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"details,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

// TransactionDetail remembers the asset's state before and after, so that a
// transaction can be undone exactly.
type TransactionDetail struct {
	ID            uint         `gorm:"primaryKey" json:"id"`
	TransactionID uint         `gorm:"not null;index" json:"transaction_id"`
	AssetID       uint         `gorm:"not null;index" json:"asset_id"`
	PrevStatus    string       `gorm:"size:20;not null" json:"prev_status"`
	PrevHolderID  *uint        `json:"prev_holder_id"`
	PrevLocation  *string      `gorm:"size:255" json:"prev_location"`
	NewStatus     string       `gorm:"size:20;not null" json:"new_status"`
	NewLocation   *string      `gorm:"size:255" json:"new_location"`
	Note          *string      `gorm:"size:255" json:"note"`
	Transaction   *Transaction `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"transaction,omitempty"`
	Asset         *Asset       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"asset,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
}

func (TransactionDetail) TableName() string { return "transaction_details" }

// AssetLog is the audit trail: every change to an asset leaves an entry.
type AssetLog struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	AssetID       uint      `gorm:"not null;index" json:"asset_id"`
	UserID        *uint     `json:"user_id"`
	Action        string    `gorm:"size:20;not null;index" json:"action"`
	FromStatus    *string   `gorm:"size:20" json:"from_status"`
	ToStatus      *string   `gorm:"size:20" json:"to_status"`
	Message       *string   `gorm:"type:text" json:"message"`
	TransactionID *uint     `json:"transaction_id"`
	Asset         *Asset    `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"asset,omitempty"`
	User          *User     `gorm:"constraint:OnDelete:SET NULL" json:"user,omitempty"`
	CreatedAt     time.Time `gorm:"index" json:"created_at"`
}

func (AssetLog) TableName() string { return "asset_logs" }

// All lists every model in dependency order for AutoMigrate.
func All() []interface{} {
	return []interface{}{
		&User{}, &RefreshToken{}, &PasswordReset{}, &ApprovalLink{}, &AuditLog{}, &Employee{}, &Category{}, &Asset{}, &AssetImage{},
		&Transaction{}, &TransactionDetail{}, &AssetLog{}, &Inspection{},
	}
}
