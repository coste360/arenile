package auth

import "slices"

// Role is a staff role (spec 16.1).
type Role string

const (
	RoleOwner      Role = "owner"
	RoleManager    Role = "manager"
	RoleCashier    Role = "cashier"
	RoleBeachStaff Role = "beach_staff"
	RoleViewer     Role = "viewer"
)

// Roles lists every role, most privileged first.
var Roles = []Role{RoleOwner, RoleManager, RoleCashier, RoleBeachStaff, RoleViewer}

// Valid reports whether r is a known role.
func (r Role) Valid() bool { return slices.Contains(Roles, r) }

// Permission is an action guarded by the permission matrix. Values are
// stable: they are exposed through the API and used as API key scopes.
type Permission string

const (
	// PermEstablishmentConfigure: establishment configuration, prices, policies.
	PermEstablishmentConfigure Permission = "establishment.configure"
	// PermUsersManage: users, roles and API keys.
	PermUsersManage Permission = "users.manage"
	// PermPaymentsConfigure: payment providers and their credentials.
	PermPaymentsConfigure Permission = "payments.configure"
	// PermBookingsWrite: create and modify bookings.
	PermBookingsWrite Permission = "bookings.write"
	// PermCashOperate: payments collection and cash sessions.
	PermCashOperate Permission = "cash.operate"
	// PermRefundsIssue: refunds.
	PermRefundsIssue Permission = "refunds.issue"
	// PermOperationsRun: check-in, releases, spot blocks.
	PermOperationsRun Permission = "operations.run"
	// PermBookingsRead: view map and bookings.
	PermBookingsRead Permission = "bookings.read"
	// PermReportsRead: reports and exports.
	PermReportsRead Permission = "reports.read"
)

// Permissions lists every permission in matrix order.
var Permissions = []Permission{
	PermEstablishmentConfigure, PermUsersManage, PermPaymentsConfigure, PermBookingsWrite,
	PermCashOperate, PermRefundsIssue, PermOperationsRun, PermBookingsRead, PermReportsRead,
}

// Valid reports whether p is a known permission.
func (p Permission) Valid() bool { return slices.Contains(Permissions, p) }

// matrix is the permission matrix of spec 16.1.
var matrix = map[Role][]Permission{
	RoleOwner: {
		PermEstablishmentConfigure, PermUsersManage, PermPaymentsConfigure, PermBookingsWrite,
		PermCashOperate, PermRefundsIssue, PermOperationsRun, PermBookingsRead, PermReportsRead,
	},
	RoleManager: {
		PermEstablishmentConfigure, PermBookingsWrite, PermCashOperate, PermRefundsIssue,
		PermOperationsRun, PermBookingsRead, PermReportsRead,
	},
	RoleCashier:    {PermBookingsWrite, PermCashOperate, PermOperationsRun, PermBookingsRead},
	RoleBeachStaff: {PermOperationsRun, PermBookingsRead},
	RoleViewer:     {PermBookingsRead, PermReportsRead},
}

// Can reports whether role grants perm. Unknown roles grant nothing.
func Can(role Role, perm Permission) bool {
	return slices.Contains(matrix[role], perm)
}

// RolePermissions returns the permissions granted by role.
func RolePermissions(role Role) []Permission {
	return slices.Clone(matrix[role])
}
