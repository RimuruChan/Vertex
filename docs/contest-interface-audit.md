# Contest interface boundary review

Scope: contest list/detail/statements, scoreboard public/internal views, submission
list/detail/progress and filters, rejudging, clarification audiences, staff/access
operations, practice problem/profile statistics, and legacy/domain-scoped routes.

## Changes

- Contest metadata visibility now applies in the service before serialization.
  Before the end, with the setting disabled, contest problem list/detail responses
  omit `difficulty` and `tags`, including staff reads through these endpoints.
  Redaction copies objects so later authorized reads retain the stored values.
- `feedback=none` withholds the public scoreboard until the contest ends. An
  untrusted `view=jury` parameter does not override caller permissions. Authorized
  staff can still explicitly request the internal board.
- Restricted submission feedback also removes `judgedAt`, in addition to the
  existing verdict/score/test-result/resource-usage redaction rules.
- Mock responses follow the same rules; tests assert absent fields in payloads,
  rather than checking CSS or hidden UI elements.

## Reviewed existing enforcement

- Submission list, detail and progress apply server-side feedback policies;
  no-feedback status filtering operates on visible verdicts before counting and
  pagination. Source access is separately checked.
- Frozen scoreboard cells and aggregate scores are projected server-side.
- Private clarification replies are scoped to their author/recipient; replies
  inherit the original recipient and cross-contest parent references are rejected.
- Observer read privileges do not grant submission, reply, rejudge or access
  mutations. Repository writes recheck resource permissions under locks.
- Domain scope and resource authorization also apply to legacy/global routes.
- Practice/profile statistics exclude contest submissions, preventing contest
  verdicts from leaking through practice completion counters.

## Boundary

Peer record access is configured independently from the scoreboard: own only,
after the end, or during the contest. Peer source is always hidden while the
contest runs and may only be shared after both the end and unfreeze. Frozen peer
submissions are either absent or projected to Pending, according to settings;
the Pending projection also applies before status filtering/counting. Own and
authorized staff submissions retain their normal access/feedback rules.

A contest display setting does not revoke independent access to an already-public
practice problem, its tags, or published editorials. For unpublished contest
material, use private/unpublished problems and contest-scoped access; copying a
public problem into a contest cannot make previously public information secret.

## Validation

Response serialization and mock-role regression tests cover metadata omission,
explicit opt-in, post-contest restoration and no-feedback public-board denial.
The complete server suite, including PostgreSQL integration tests, runs against
an isolated database. Go tests run inside the same Podman VM as PostgreSQL to
avoid host/VM clock skew affecting expiration/lease tests.
