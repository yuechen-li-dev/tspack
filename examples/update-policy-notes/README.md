# Update Policy Notes

A realistic TypeScript workspace fixture for dogfooding declared rolling update policy without relying on external update bots.

The fixture intentionally uses registry-style dependencies plus workspace dependencies so tests can attach an offline fake registry with deterministic candidate versions and lifecycle metadata.
