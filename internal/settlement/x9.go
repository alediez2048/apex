package settlement

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/alediez2048/apex/internal/domain"
)

// FileHeader is the X9-style file header.
// Routing numbers are demo defaults; in production these would come from config.
type FileHeader struct {
	StandardLevel      string `json:"standard_level"`
	DestinationRouting string `json:"destination_routing"`
	OriginRouting      string `json:"origin_routing"`
	FileCreationDate   string `json:"file_creation_date"` // YYYYMMDD
	FileCreationTime   string `json:"file_creation_time"` // HHMM
	SettlementDate     string `json:"settlement_date"`    // YYYY-MM-DD business day for this batch
}

// CheckDetail is the check detail record (amount in cents, MICR).
type CheckDetail struct {
	Amount          int64  `json:"amount"`
	MICRRouting     string `json:"micr_routing"`
	MICROnUs        string `json:"micr_on_us"`
	PayorBankRouting string `json:"payor_bank_routing"`
	ItemSequence    string `json:"item_sequence"`
}

// ImageView references front/back image.
type ImageView struct {
	Side string `json:"side"`
	Ref  string `json:"ref"`
}

// CheckRecord is one check with detail and image views.
type CheckRecord struct {
	Detail     CheckDetail `json:"detail"`
	ImageViews []ImageView `json:"image_views"`
}

// BundleHeader identifies a bundle.
type BundleHeader struct {
	BundleID string `json:"bundle_id"`
}

// Bundle is a bundle of checks.
type Bundle struct {
	Header BundleHeader  `json:"header"`
	Checks []CheckRecord `json:"checks"`
}

// CashLetterHeader is the cash letter header.
type CashLetterHeader struct {
	CollectionType      string `json:"collection_type"`
	DestinationRouting  string `json:"destination_routing"`
	RecordType          string `json:"record_type"`
}

// CashLetter contains bundles.
type CashLetter struct {
	Header  CashLetterHeader `json:"header"`
	Bundles []Bundle         `json:"bundles"`
}

// FileControl is the file-level control totals.
type FileControl struct {
	TotalAmount  int64 `json:"total_amount"`
	ItemCount    int   `json:"item_count"`
	BundleCount  int   `json:"bundle_count"`
}

// X9File is the full X9 ICL-structured JSON.
type X9File struct {
	FileHeader  FileHeader   `json:"file_header"`
	CashLetters []CashLetter `json:"cash_letters"`
	FileControl FileControl  `json:"file_control"`
}

// BuildFile builds the X9-structured JSON from a list of FundsPosted transfers.
// fileCreation is used for file_header date/time; settlementDate is YYYY-MM-DD for reference.
func BuildFile(transfers []*domain.Transfer, settlementDate string, fileCreation time.Time) (*X9File, error) {
	if len(transfers) == 0 {
		return nil, fmt.Errorf("settlement: no transfers to build file")
	}
	in := fileCreation.UTC()
	dateStr := in.Format("20060102")
	timeStr := in.Format("1504")

	var totalAmount int64
	checks := make([]CheckRecord, 0, len(transfers))
	for i, t := range transfers {
		totalAmount += int64(t.Amount)
		onUs := t.MICRAccount
		if onUs == "" {
			onUs = t.CheckNumber
		}
		if t.MICRRouting != "" && onUs == "" {
			onUs = t.MICRRouting
		}
		seq := fmt.Sprintf("%09d", i+1)
		views := []ImageView{
			{Side: "front", Ref: "/api/v1/deposits/" + t.ID + "/images/front"},
			{Side: "back", Ref: "/api/v1/deposits/" + t.ID + "/images/back"},
		}
		checks = append(checks, CheckRecord{
			Detail: CheckDetail{
				Amount:           int64(t.Amount),
				MICRRouting:      t.MICRRouting,
				MICROnUs:         onUs,
				PayorBankRouting: t.MICRRouting,
				ItemSequence:     seq,
			},
			ImageViews: views,
		})
	}

	f := &X9File{
		FileHeader: FileHeader{
			StandardLevel:      "03",
			DestinationRouting: "021000021",
			OriginRouting:      "061000052",
			FileCreationDate:   dateStr,
			FileCreationTime:   timeStr,
			SettlementDate:     settlementDate,
		},
		CashLetters: []CashLetter{
			{
				Header: CashLetterHeader{
					CollectionType:     "01",
					DestinationRouting: "021000021",
					RecordType:         "10",
				},
				Bundles: []Bundle{
					{
						Header: BundleHeader{BundleID: "001"},
						Checks: checks,
					},
				},
			},
		},
		FileControl: FileControl{
			TotalAmount: totalAmount,
			ItemCount:   len(transfers),
			BundleCount: 1,
		},
	}
	return f, nil
}

// ToJSON marshals the X9 file to JSON string.
func (f *X9File) ToJSON() (string, error) {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
