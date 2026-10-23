package device

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// maxCodeAttempts bounds the redraws on a code_hash collision (40-bit
// codes: a collision among live codes is already improbable).
const maxCodeAttempts = 3

// CreatePairingCode creates a single-use code for in.UserID (default the
// caller) on establishmentID. Audit "device.pairing_code_created".
//
// Only people pair devices: API keys (no user to bind the token to) and
// device principals (a stolen token must not be able to mint new ones)
// get ErrForbidden. Any role may pair a device for itself; a code for
// another user needs auth.PermUsersManage and that user must be active
// with a role on the establishment.
func (s *Service) CreatePairingCode(ctx context.Context, p *auth.Principal, establishmentID string, in PairingCodeInput) (*PairingCode, error) {
	if p == nil {
		return nil, auth.ErrUnauthenticated
	}
	if p.Kind != auth.PrincipalUser || p.DeviceID != "" {
		return nil, auth.ErrForbidden
	}
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	ttl, err := pairingTTL(in.TTL)
	if err != nil {
		return nil, err
	}
	if in.Kind != "" && !ValidKind(in.Kind) {
		return nil, configerr.Invalid("kind", "tipo di dispositivo non valido")
	}
	userID := in.UserID
	if userID == "" {
		userID = p.UserID
	}
	if userID != p.UserID && !p.Can(es, auth.PermUsersManage) {
		return nil, auth.ErrForbidden
	}
	if s.d.Instance == nil {
		return nil, ErrLANModeRequired
	}
	target, err := s.d.Instance.PairingTarget(ctx)
	if err != nil {
		return nil, err
	}

	now := s.d.Clock.Now().UTC()
	out := &PairingCode{ExpiresAt: now.Add(ttl), EstablishmentID: es.ID(), UserID: userID}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		grants, err := store.ListTeamGrants(ctx, tx, es)
		if err != nil {
			return err
		}
		if !activeMember(grants, userID) {
			return configerr.Invalid("user_id", "l'utente non è attivo o non ha un ruolo in questo stabilimento")
		}
		est, err := store.GetEstablishment(ctx, tx, es)
		if err != nil {
			return err
		}
		row := &store.DevicePairingCode{
			ID: store.NewID(), UserID: userID, CreatedByUserID: &p.UserID,
			ExpiresAt: out.ExpiresAt, CreatedAt: now,
		}
		if in.Kind != "" {
			k := in.Kind
			row.Kind = &k
		}
		for attempt := 1; ; attempt++ {
			code, err := NewPairingCode()
			if err != nil {
				return err
			}
			row.CodeHash, _ = HashPairingCode(code)
			// A savepoint keeps the transaction usable after a collision
			// (PostgreSQL aborts the whole transaction on any error).
			err = tx.RunInTx(ctx, nil, func(ctx context.Context, sp bun.Tx) error {
				return store.InsertPairingCode(ctx, sp, es, row)
			})
			if err == nil {
				out.Code = code
				break
			}
			if !errors.Is(err, store.ErrConflict) || attempt == maxCodeAttempts {
				return err
			}
		}
		out.QR = QRPayload{V: QRVersion, URL: target.URL, Code: out.Code, Fingerprint: target.Fingerprint, EstablishmentName: est.Name}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "device.pairing_code_created",
			EntityType: "device_pairing_code", EntityID: row.ID,
			Diff: map[string]any{"user_id": userID, "kind": in.Kind, "expires_at": out.ExpiresAt},
		})
	})
	if err != nil {
		return nil, fmt.Errorf("device: create pairing code: %w", err)
	}
	if out.QRText, err = EncodeQR(out.QR); err != nil {
		return nil, fmt.Errorf("device: encode qr: %w", err)
	}
	if svg, err := RenderQRSVG(out.QRText); err == nil {
		out.QRSVG = svg
	} else {
		s.d.Log.WarnContext(ctx, "device: pairing qr not rendered", slog.String("error", err.Error()))
	}
	return out, nil
}

// Pair exchanges a code for a device token: conditional UPDATE consuming
// the code, membership check of the code's user, insert of the device and
// audit "device.paired", in one transaction. Unknown, expired and used
// codes, and codes of users who lost access meanwhile, are all
// ErrPairingCodeInvalid (no oracle). The token is returned once.
func (s *Service) Pair(ctx context.Context, in PairInput) (*PairResult, error) {
	codeHash, ok := HashPairingCode(in.Code)
	if !ok {
		return nil, ErrPairingCodeInvalid
	}
	in, err := normalizePairInput(in)
	if err != nil {
		return nil, err
	}
	token, prefix, err := NewToken()
	if err != nil {
		return nil, err
	}
	now := s.d.Clock.Now().UTC()
	var view View
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		code, err := store.ConsumePairingCode(ctx, tx, codeHash, now)
		if errors.Is(err, store.ErrNotFound) {
			return ErrPairingCodeInvalid
		}
		if err != nil {
			return err
		}
		es, err := store.SystemEstablishmentScope(ctx, tx, code.EstablishmentID)
		if err != nil {
			return ErrPairingCodeInvalid
		}
		grants, err := store.ListTeamGrants(ctx, tx, es)
		if err != nil {
			return err
		}
		role, hasRole := effectiveRole(grants, code.UserID)
		if !hasRole || !activeMember(grants, code.UserID) {
			return ErrPairingCodeInvalid
		}
		user, err := store.GetUser(ctx, tx, code.UserID)
		if err != nil {
			return err
		}
		d := store.Device{
			ID: store.NewID(), UserID: code.UserID, Name: in.Name, Platform: in.Platform, Kind: in.Kind,
			TokenPrefix: prefix, TokenHash: HashToken(token), PairingCodeID: &code.ID,
			CreatedAt: now, UpdatedAt: now,
		}
		if in.AppVersion != "" {
			v := in.AppVersion
			d.AppVersion = &v
		}
		if err := store.InsertDevice(ctx, tx, es, &d); err != nil {
			return err
		}
		if err := store.SetPairingCodeDevice(ctx, tx, code.ID, d.ID); err != nil {
			return err
		}
		view = View{Device: d, UserName: user.Name, Role: role, Current: true}
		// The code proves the intent of its user: the user is the actor.
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: audit.Actor{Type: audit.ActorUser, ID: code.UserID, IP: in.IP},
			Action: "device.paired", EntityType: "device", EntityID: d.ID,
			Diff: map[string]any{
				"name": d.Name, "platform": d.Platform, "kind": d.Kind, "app_version": in.AppVersion,
				"token_prefix": DisplayPrefix(prefix), "pairing_code_id": code.ID,
			},
		})
	})
	if err != nil {
		if errors.Is(err, ErrPairingCodeInvalid) {
			return nil, ErrPairingCodeInvalid
		}
		return nil, fmt.Errorf("device: pair: %w", err)
	}
	return &PairResult{Token: token, Device: view}, nil
}
