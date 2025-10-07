package version

import (
	"fmt"
)

// SMPPVersion represents an SMPP protocol version
type SMPPVersion uint8

const (
	// SMPPVersion33 represents SMPP v3.3
	SMPPVersion33 SMPPVersion = 0x33
	// SMPPVersion34 represents SMPP v3.4
	SMPPVersion34 SMPPVersion = 0x34
	// SMPPVersion50 represents SMPP v5.0
	SMPPVersion50 SMPPVersion = 0x50
)

// String returns the string representation of the version
func (v SMPPVersion) String() string {
	switch v {
	case SMPPVersion33:
		return "3.3"
	case SMPPVersion34:
		return "3.4"
	case SMPPVersion50:
		return "5.0"
	default:
		return fmt.Sprintf("unknown (%02x)", uint8(v))
	}
}

// IsSupported checks if the version is supported
func (v SMPPVersion) IsSupported() bool {
	switch v {
	case SMPPVersion33, SMPPVersion34, SMPPVersion50:
		return true
	default:
		return false
	}
}

// SupportsFeature checks if the version supports a specific feature
func (v SMPPVersion) SupportsFeature(feature string) bool {
	switch feature {
	case "broadcast_sm":
		return v >= SMPPVersion34
	case "cancel_broadcast_sm":
		return v >= SMPPVersion34
	case "query_broadcast_sm":
		return v >= SMPPVersion34
	case "data_sm":
		return v >= SMPPVersion34
	case "outbind":
		return v >= SMPPVersion34
	case "replace_sm":
		return v >= SMPPVersion34
	case "cancel_sm":
		return v >= SMPPVersion34
	case "submit_multi":
		return v >= SMPPVersion34
	case "alert_notification":
		return v >= SMPPVersion34
	case "enquire_link":
		return v >= SMPPVersion33
	case "unbind":
		return v >= SMPPVersion33
	default:
		return false
	}
}

// NegotiateVersion negotiates the highest supported version between client and server
func NegotiateVersion(clientVersion, serverVersion SMPPVersion) (SMPPVersion, error) {
	// If versions match, use that version
	if clientVersion == serverVersion {
		return clientVersion, nil
	}

	// Server dictates the version, but we can negotiate down
	if clientVersion > serverVersion {
		// Client wants newer version, server supports older
		if serverVersion.IsSupported() {
			return serverVersion, nil
		}
	} else {
		// Client wants older version, server supports newer
		if clientVersion.IsSupported() {
			return clientVersion, nil
		}
	}

	return 0, fmt.Errorf("no compatible SMPP version found (client: %s, server: %s)",
		clientVersion.String(), serverVersion.String())
}

// VersionNegotiator handles version negotiation
type VersionNegotiator struct {
	supportedVersions []SMPPVersion
	defaultVersion    SMPPVersion
}

// NewVersionNegotiator creates a new version negotiator
func NewVersionNegotiator(defaultVersion SMPPVersion) *VersionNegotiator {
	return &VersionNegotiator{
		supportedVersions: []SMPPVersion{SMPPVersion33, SMPPVersion34, SMPPVersion50},
		defaultVersion:    defaultVersion,
	}
}

// Negotiate negotiates the version with a client
func (vn *VersionNegotiator) Negotiate(clientVersion SMPPVersion) (SMPPVersion, error) {
	return NegotiateVersion(clientVersion, vn.defaultVersion)
}

// IsVersionSupported checks if a version is supported
func (vn *VersionNegotiator) IsVersionSupported(version SMPPVersion) bool {
	for _, v := range vn.supportedVersions {
		if v == version {
			return true
		}
	}
	return false
}

// GetSupportedVersions returns all supported versions
func (vn *VersionNegotiator) GetSupportedVersions() []SMPPVersion {
	versions := make([]SMPPVersion, len(vn.supportedVersions))
	copy(versions, vn.supportedVersions)
	return versions
}

// GetDefaultVersion returns the default version
func (vn *VersionNegotiator) GetDefaultVersion() SMPPVersion {
	return vn.defaultVersion
}
