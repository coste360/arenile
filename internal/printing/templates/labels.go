package templates

// Label keys of the printed documents.
const (
	lblTicket        = "ticket"
	lblServiceDay    = "service_day"
	lblIssued        = "issued"
	lblZone          = "zone"
	lblSpot          = "spot"
	lblToAssign      = "to_assign"
	lblFormula       = "formula"
	lblTotal         = "total"
	lblVAT           = "vat"
	lblVATOn         = "vat_on"
	lblPayment       = "payment"
	lblBookingCode   = "booking_code"
	lblNonFiscal     = "non_fiscal"
	lblReceipt       = "receipt"
	lblNumber        = "number"
	lblDate          = "date"
	lblOperator      = "operator"
	lblZTitle        = "z_title"
	lblZDraft        = "z_draft"
	lblRegister      = "register"
	lblSession       = "session"
	lblOpened        = "opened"
	lblClosed        = "closed"
	lblOperators     = "operators"
	lblOpeningFloat  = "opening_float"
	lblByMethod      = "by_method"
	lblMovements     = "movements"
	lblCounted       = "counted"
	lblDifference    = "difference"
	lblExpectedTotal = "expected_total"
	lblCountedTotal  = "counted_total"
	lblTestTitle     = "test_title"
	lblPrinter       = "printer"
	lblAddress       = "address"
	lblPaper         = "paper"
	lblChars         = "chars"
	lblCodepage      = "codepage"
	lblCharset       = "charset"
	lblStyles        = "styles"
	lblBold          = "bold"
	lblDouble        = "double"
	lblColumns       = "columns"
	lblSample        = "sample"
	lblTestOK        = "test_ok"
	lblPhone         = "phone"
	lblVATNumber     = "vat_number"
	lblNone          = "none"
	lblCopy          = "copy"
)

// labels are the printed strings per language (it is the source language).
var labels = map[string]map[string]string{
	"it": {
		lblTicket: "BIGLIETTO", lblServiceDay: "Giorno", lblIssued: "Emesso", lblZone: "Zona",
		lblSpot: "Postazione", lblToAssign: "da assegnare", lblFormula: "Formula", lblTotal: "TOTALE",
		lblVAT: "IVA", lblVATOn: "su", lblPayment: "Pagamento", lblBookingCode: "Codice",
		lblNonFiscal: "Documento non fiscale", lblReceipt: "RICEVUTA NON FISCALE", lblNumber: "N.",
		lblDate: "Data", lblOperator: "Operatore", lblZTitle: "CHIUSURA DI CASSA (Z)", lblZDraft: "NON CHIUSA",
		lblRegister: "Cassa", lblSession: "Sessione", lblOpened: "Apertura", lblClosed: "Chiusura",
		lblOperators: "Operatori", lblOpeningFloat: "Fondo cassa", lblByMethod: "Atteso per metodo",
		lblMovements: "Movimenti", lblCounted: "Contato", lblDifference: "DIFFERENZA",
		lblExpectedTotal: "Totale atteso", lblCountedTotal: "Totale contato",
		lblTestTitle: "STAMPA DI PROVA", lblPrinter: "Stampante", lblAddress: "Indirizzo", lblPaper: "Carta",
		lblChars: "caratteri", lblCodepage: "Codepage", lblCharset: "Caratteri", lblStyles: "Stili",
		lblBold: "Grassetto", lblDouble: "Doppia", lblColumns: "Colonne", lblSample: "Ombrellone A12",
		lblTestOK: "Se leggi questa pagina la stampante funziona.", lblPhone: "Tel.", lblVATNumber: "P.IVA",
		lblNone: "nessuno", lblCopy: "COPIA",
	},
	"en": {
		lblTicket: "TICKET", lblServiceDay: "Day", lblIssued: "Issued", lblZone: "Zone",
		lblSpot: "Spot", lblToAssign: "to be assigned", lblFormula: "Option", lblTotal: "TOTAL",
		lblVAT: "VAT", lblVATOn: "on", lblPayment: "Payment", lblBookingCode: "Code",
		lblNonFiscal: "Not a fiscal document", lblReceipt: "NON-FISCAL RECEIPT", lblNumber: "No.",
		lblDate: "Date", lblOperator: "Operator", lblZTitle: "CASH CLOSING (Z)", lblZDraft: "NOT CLOSED",
		lblRegister: "Register", lblSession: "Session", lblOpened: "Opened", lblClosed: "Closed",
		lblOperators: "Operators", lblOpeningFloat: "Opening float", lblByMethod: "Expected by method",
		lblMovements: "Movements", lblCounted: "Counted", lblDifference: "DIFFERENCE",
		lblExpectedTotal: "Expected total", lblCountedTotal: "Counted total",
		lblTestTitle: "TEST PRINT", lblPrinter: "Printer", lblAddress: "Address", lblPaper: "Paper",
		lblChars: "characters", lblCodepage: "Codepage", lblCharset: "Characters", lblStyles: "Styles",
		lblBold: "Bold", lblDouble: "Double", lblColumns: "Columns", lblSample: "Umbrella A12",
		lblTestOK: "If you can read this page the printer works.", lblPhone: "Tel.", lblVATNumber: "VAT no.",
		lblNone: "none", lblCopy: "COPY",
	},
	"de": {
		lblTicket: "TICKET", lblServiceDay: "Tag", lblIssued: "Ausgestellt", lblZone: "Zone",
		lblSpot: "Platz", lblToAssign: "wird zugewiesen", lblFormula: "Option", lblTotal: "GESAMT",
		lblVAT: "MwSt.", lblVATOn: "auf", lblPayment: "Zahlung", lblBookingCode: "Code",
		lblNonFiscal: "Kein Steuerbeleg", lblReceipt: "QUITTUNG (KEIN STEUERBELEG)", lblNumber: "Nr.",
		lblDate: "Datum", lblOperator: "Bediener", lblZTitle: "KASSENABSCHLUSS (Z)", lblZDraft: "NICHT ABGESCHLOSSEN",
		lblRegister: "Kasse", lblSession: "Sitzung", lblOpened: "Geöffnet", lblClosed: "Geschlossen",
		lblOperators: "Bediener", lblOpeningFloat: "Wechselgeld", lblByMethod: "Soll pro Zahlungsart",
		lblMovements: "Bewegungen", lblCounted: "Gezählt", lblDifference: "DIFFERENZ",
		lblExpectedTotal: "Soll gesamt", lblCountedTotal: "Gezählt gesamt",
		lblTestTitle: "TESTDRUCK", lblPrinter: "Drucker", lblAddress: "Adresse", lblPaper: "Papier",
		lblChars: "Zeichen", lblCodepage: "Codepage", lblCharset: "Zeichen", lblStyles: "Stile",
		lblBold: "Fett", lblDouble: "Doppelt", lblColumns: "Spalten", lblSample: "Schirm A12",
		lblTestOK: "Wenn Sie diese Seite lesen, funktioniert der Drucker.", lblPhone: "Tel.", lblVATNumber: "USt-IdNr.",
		lblNone: "keine", lblCopy: "KOPIE",
	},
	"fr": {
		lblTicket: "BILLET", lblServiceDay: "Jour", lblIssued: "Émis", lblZone: "Zone",
		lblSpot: "Emplacement", lblToAssign: "à attribuer", lblFormula: "Formule", lblTotal: "TOTAL",
		lblVAT: "TVA", lblVATOn: "sur", lblPayment: "Paiement", lblBookingCode: "Code",
		lblNonFiscal: "Document non fiscal", lblReceipt: "REÇU NON FISCAL", lblNumber: "N°",
		lblDate: "Date", lblOperator: "Opérateur", lblZTitle: "CLÔTURE DE CAISSE (Z)", lblZDraft: "NON CLÔTURÉE",
		lblRegister: "Caisse", lblSession: "Session", lblOpened: "Ouverture", lblClosed: "Clôture",
		lblOperators: "Opérateurs", lblOpeningFloat: "Fond de caisse", lblByMethod: "Attendu par moyen",
		lblMovements: "Mouvements", lblCounted: "Compté", lblDifference: "ÉCART",
		lblExpectedTotal: "Total attendu", lblCountedTotal: "Total compté",
		lblTestTitle: "PAGE DE TEST", lblPrinter: "Imprimante", lblAddress: "Adresse", lblPaper: "Papier",
		lblChars: "caractères", lblCodepage: "Codepage", lblCharset: "Caractères", lblStyles: "Styles",
		lblBold: "Gras", lblDouble: "Double", lblColumns: "Colonnes", lblSample: "Parasol A12",
		lblTestOK: "Si vous lisez cette page, l'imprimante fonctionne.", lblPhone: "Tél.", lblVATNumber: "N° TVA",
		lblNone: "aucun", lblCopy: "COPIE",
	},
}

// MethodLabel is the printed name of a payment method code in lang
// (unknown codes are printed as they are).
func MethodLabel(lang, method string) string {
	m, ok := methodLabels[normLang(lang)][method]
	if !ok {
		return method
	}
	return m
}

var methodLabels = map[string]map[string]string{
	"it": {"cash": "Contanti", "card": "Carta", "bank_transfer": "Bonifico", "wallet": "Wallet", "credit": "Credito", "other": "Altro"},
	"en": {"cash": "Cash", "card": "Card", "bank_transfer": "Bank transfer", "wallet": "Wallet", "credit": "Credit", "other": "Other"},
	"de": {"cash": "Bar", "card": "Karte", "bank_transfer": "Überweisung", "wallet": "Wallet", "credit": "Guthaben", "other": "Sonstiges"},
	"fr": {"cash": "Espèces", "card": "Carte", "bank_transfer": "Virement", "wallet": "Wallet", "credit": "Avoir", "other": "Autre"},
}

func normLang(lang string) string {
	if len(lang) >= 2 {
		if _, ok := labels[lang[:2]]; ok {
			return lang[:2]
		}
	}
	return "it"
}

func label(lang, key string) string { return labels[normLang(lang)][key] }
