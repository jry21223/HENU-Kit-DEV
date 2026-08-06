package practice

// UpdateRankingProfilePath mirrors quizcraft.yaml's updateRankingProfile
// operation. The curated quizcraftcontractgen only emits internal
// /api/v1/portal/practice/ POST commands, and this PATCH route uses the public
// userSession security model, so the generator does not emit it yet. Keep it
// declared alongside the generated contract; cutover should fold it into
// cmd/quizcraftcontractgen once the generator grows a user-route write mode.
const UpdateRankingProfilePath = "/api/v1/ranking-profile"
