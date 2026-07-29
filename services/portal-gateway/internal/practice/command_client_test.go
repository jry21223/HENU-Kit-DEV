package practice

import "testing"

const validPracticeSessionEnvelope = `{"request_id":"req_core_session","data":{"session_id":"22222222-2222-4222-8222-222222222222","bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-4444-8444-444444444444","mode":"random","excluded_unavailable_count":0,"questions":[{"question_id":"55555555-5555-4555-8555-555555555555","question_version_id":"66666666-6666-4666-8666-666666666666","type":"single","chapter_id":"ch01","chapter":"基础","content":"服务端选择的题目","options":["甲","乙"]}]}}`

const validPracticeAnswerEnvelope = `{"request_id":"req_core_answer","data":{"question_id":"55555555-5555-4555-8555-555555555555","question_version_id":"66666666-6666-4666-8666-666666666666","correct":false,"replayed":false,"expected_answer":1,"analysis":"服务端讲解"}}`

func TestValidatePracticeSessionEnvelopeRejectsIncompleteOrUnclosedCoreData(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "missing excluded unavailable count",
			raw:  `{"request_id":"req_core_session","data":{"session_id":"22222222-2222-4222-8222-222222222222","bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-8444-8444-444444444444","mode":"random","questions":[]}}`,
		},
		{
			name: "missing required question chapter",
			raw:  `{"request_id":"req_core_session","data":{"session_id":"22222222-2222-4222-8222-222222222222","bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-8444-8444-444444444444","mode":"random","excluded_unavailable_count":0,"questions":[{"question_id":"55555555-5555-4555-8555-555555555555","question_version_id":"66666666-6666-4666-8666-666666666666","type":"single","chapter_id":"ch01","content":"服务端选择的题目"}]}}`,
		},
		{
			name: "question answer leaks before submission",
			raw:  `{"request_id":"req_core_session","data":{"session_id":"22222222-2222-4222-8222-222222222222","bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-8444-8444-444444444444","mode":"random","excluded_unavailable_count":0,"questions":[{"question_id":"55555555-5555-4555-8555-555555555555","question_version_id":"66666666-6666-4666-8666-666666666666","type":"single","chapter_id":"ch01","chapter":"基础","content":"服务端选择的题目","answer":1}]}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validatePracticeSessionEnvelope([]byte(test.raw)); err == nil {
				t.Fatal("accepted an incomplete or unclosed Core session response")
			}
		})
	}
	if err := validatePracticeSessionEnvelope([]byte(validPracticeSessionEnvelope)); err != nil {
		t.Fatalf("rejected valid Core session response: %v", err)
	}
}

func TestValidatePracticeAnswerEnvelopeRejectsIncompleteOrUnclosedCoreData(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "missing required analysis",
			raw:  `{"request_id":"req_core_answer","data":{"question_id":"55555555-5555-4555-8555-555555555555","question_version_id":"66666666-6666-4666-8666-666666666666","correct":false,"replayed":false,"expected_answer":1}}`,
		},
		{
			name: "unknown answer result field",
			raw:  `{"request_id":"req_core_answer","data":{"question_id":"55555555-5555-4555-8555-555555555555","question_version_id":"66666666-6666-4666-8666-666666666666","correct":false,"replayed":false,"expected_answer":1,"analysis":"服务端讲解","score":1}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validatePracticeAnswerEnvelope([]byte(test.raw)); err == nil {
				t.Fatal("accepted an incomplete or unclosed Core answer response")
			}
		})
	}
	if err := validatePracticeAnswerEnvelope([]byte(validPracticeAnswerEnvelope)); err != nil {
		t.Fatalf("rejected valid Core answer response: %v", err)
	}
}
