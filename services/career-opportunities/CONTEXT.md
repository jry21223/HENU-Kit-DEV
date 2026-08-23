# Career Opportunities Context

Career Opportunities is the HENUKit product context for finding and following public internship and campus-recruitment opportunities. It owns opportunity facts and user career intent without becoming an applicant, recruitment platform, or membership authority.

## Language

**Job Opportunity**:
A normalized, source-attributed opening observed on an approved public recruitment source, with explicit freshness and availability state.
_Avoid_: Guaranteed vacancy, scraped job, application

**Public Job Catalog**:
The current collection of Job Opportunities that anyone may browse and search without a Membership Entitlement.
_Avoid_: Free trial, member inventory, complete market

**Career Profile**:
A user's structured directions, skills, locations, job type, availability, and matching keywords. It is not a resume file or a Platform Core identity profile.
_Avoid_: Resume, account profile, prompt

**Resume Text**:
The user-reviewed narrative of project, competition, and internship experience inside a Career Profile. It may be entered directly or produced by Resume Extraction, but it is not the uploaded resume file.
_Avoid_: Original resume, stored resume file, verified experience

**Resume Extraction**:
A temporary processing job that turns a user-uploaded resume into a Career Profile draft. Uploaded file bytes are discarded after processing; the draft remains editable and is not saved until the user confirms it.
_Avoid_: Saved resume, automatic profile update, permanent upload

**Suification Draft**:
An entertainment-only rewrite of the current Resume Text. It may intensify wording without inventing facts and remains temporary until the user applies it to the editable Career Profile.
_Avoid_: Resume optimization, verified resume, automatic save

**Opportunity Match**:
A versioned, explainable comparison between one Career Profile and one Job Opportunity. It is guidance, not an eligibility decision or employment prediction.
_Avoid_: AI recommendation, hiring score, acceptance probability

**Opportunity Tracking**:
A member-owned record that follows a Job Opportunity as saved, preparing to apply, applied, or not interested without submitting an application.
_Avoid_: Application, delivery record, recruitment status

**Opportunity Digest**:
An explicitly enabled summary of new or changed Opportunity Matches delivered through an approved HENUKit channel.
_Avoid_: Marketing blast, arbitrary email, crawler report

**Career Benefit**:
The Career Profile, Opportunity Match, Opportunity Tracking, history, and Opportunity Digest capabilities granted by an Account Portfolio `lifetime` Membership Entitlement. The Public Job Catalog is not a Career Benefit and remains publicly readable.
_Avoid_: Subscription plan, GetWork account, paid job data
