# Account Portfolio Gateway integration

Run `go test -race -count=1 ./...` from this directory. With no database URL,
the test starts and cleans up one isolated PostgreSQL 16 Testcontainer. CI sets
`ACCOUNT_GATEWAY_INTEGRATION_DATABASE_URL` to a dedicated, fresh PostgreSQL 17
database; migrations and all fixture writes remain inside that test database.

The Gateway HTTP handler, signed owner client, Account Portfolio HTTP owner,
migrations, commands and stored results are real. Platform Core authorization
and identity HTTP responses are explicit fixtures, so this is not evidence of
real Core permission persistence or production behavior. Redis is not contacted
by these account routes.

The test covers original-key recovery after a discarded write response, an
older committed result after the current ticket advances, actor/operation/key
isolation, exact product Scope and permission forwarding, identity removal at
the browser boundary, notifications, idempotent replay and concurrent revisions.

`TestMembershipAndPointsRecoverOriginalFactsThroughGateway` adds real TCP-loss
recovery for grants and point adjustments, revocation recovery, original results
after later entitlement/balance changes, same-key payload/target conflicts,
uninitialized membership 404s, concurrent grants and debits, rejected keys that
remain unknown, and public owner ledger/notification checks. It uses the same
explicit Core authorization HTTP fixture boundary, not a Core database. Each
test starts its own temporary database when no dedicated CI URL is provided;
they run sequentially unless the runner is explicitly changed.
