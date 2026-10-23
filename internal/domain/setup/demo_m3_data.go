package setup

// Fake people of the demo (spec 27.10): Italian names, example.com emails
// and +39 000 numbers that cannot reach anyone. Never use real contacts.

var demoFirstNames = []string{
	"Giulia", "Marco", "Francesca", "Luca", "Chiara", "Alessandro", "Sara", "Davide",
	"Martina", "Matteo", "Valentina", "Andrea", "Elena", "Simone", "Federica", "Lorenzo",
	"Alessia", "Stefano", "Silvia", "Riccardo",
}

var demoLastNames = []string{
	"Rossi", "Russo", "Ferrari", "Esposito", "Bianchi", "Romano", "Colombo", "Ricci",
	"Marino", "Greco", "Bruno", "Gallo", "Conti", "De Luca", "Mancini", "Costa",
	"Giordano", "Rizzo", "Lombardi", "Moretti", "Barbieri", "Fontana", "Santoro", "Mariani",
	"Rinaldi", "Caruso", "Ferrara", "Galli", "Martini", "Leone", "Longo", "Gentile",
	"Martinelli", "Vitale", "Lombardo", "Serra", "Coppola", "De Santis", "D'Angelo", "Marchetti",
}

// demoLocales cycles the customer locales (mostly Italian guests).
var demoLocales = []string{"it", "it", "it", "it", "en", "it", "de", "it", "fr", "it"}

// demoTags cycles the customer tags.
var demoTags = [][]string{{"demo"}, {"demo", "abituale"}, {"demo"}, {"demo", "famiglia"}, {"demo"}}
