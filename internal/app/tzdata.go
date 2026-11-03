package app

import (
	// Embed the IANA time zone database in every binary built on the
	// composition root (server and desktop): Windows and minimal images have no
	// system zoneinfo, and without it every establishment time zone,
	// Europe/Rome included, fails time.LoadLocation.
	_ "time/tzdata"
)
