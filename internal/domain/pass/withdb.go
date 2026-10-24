package pass

import "github.com/uptrace/bun"

// WithDB returns a copy of the service bound to db (composition in a caller
// transaction, e.g. the CSV import of season passes: the service's own
// transactions become savepoints). Added for internal/domain/importer.
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}
