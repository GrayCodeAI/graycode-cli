package scoring

import (
	"math"
	"testing"
)

// almostEqual compares floats with a small tolerance for BM25 arithmetic.
func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestNewBM25Scorer_Defaults(t *testing.T) {
	s := NewBM25Scorer(0, 0)
	if s.K1 != DefaultBM25K1 {
		t.Errorf("K1 = %v, want default %v", s.K1, DefaultBM25K1)
	}
	if s.B != DefaultBM25B {
		t.Errorf("B = %v, want default %v", s.B, DefaultBM25B)
	}
}

func TestNewBM25Scorer_CustomValues(t *testing.T) {
	s := NewBM25Scorer(2.0, 0.5)
	if s.K1 != 2.0 || s.B != 0.5 {
		t.Errorf("K1/B = %v/%v, want 2.0/0.5", s.K1, s.B)
	}
}

func TestBM25Score(t *testing.T) {
	tests := []struct {
		name      string
		scorer    *BM25Scorer
		query     []string
		tf        map[string]int
		docLen    float64
		avgDocLen float64
		docCount  int
		docFreq   map[string]int
		want      float64
	}{
		{
			name:      "single term, exact formula",
			scorer:    NewBM25Scorer(1.2, 0.75),
			query:     []string{"hello"},
			tf:        map[string]int{"hello": 1},
			docLen:    10,
			avgDocLen: 10,
			docCount:  10,
			docFreq:   map[string]int{"hello": 2},
			// idf = ln((10-2+0.5)/(2+0.5) + 1) = ln(8.5/2.5+1) = ln(4.4)
			// numerator = 1*(1.2+1) = 2.2
			// denominator = 1 + 1.2*(1-0.75+0.75*(10/10)) = 1 + 1.2*1 = 2.2
			// score = idf * 2.2/2.2 = idf = ln(4.4)
			want: math.Log(4.4),
		},
		{
			name:      "term not in doc contributes zero",
			scorer:    NewBM25Scorer(1.2, 0.75),
			query:     []string{"absent"},
			tf:        map[string]int{"present": 3},
			docLen:    10,
			avgDocLen: 10,
			docCount:  5,
			docFreq:   map[string]int{"absent": 1},
			want:      0,
		},
		{
			name:      "multi-term sums independent contributions",
			scorer:    NewBM25Scorer(1.2, 0.75),
			query:     []string{"a", "b"},
			tf:        map[string]int{"a": 1, "b": 2},
			docLen:    5,
			avgDocLen: 5,
			docCount:  4,
			docFreq:   map[string]int{"a": 1, "b": 1},
			// idf = ln((4-1+0.5)/(1+0.5)+1) = ln(3.5/1.5+1) = ln(3.333); length factor = 1.
			// term a: idf * (1*2.2)/(1 + 1.2*1) = idf * 2.2/2.2 = idf
			// term b: idf * (2*2.2)/(2 + 1.2*1) = idf * 4.4/3.2
			want: math.Log(10.0/3.0) * (1 + 4.4/3.2),
		},
		{
			name:      "higher tf yields higher score",
			scorer:    NewBM25Scorer(1.2, 0.75),
			query:     []string{"t"},
			tf:        map[string]int{"t": 5},
			docLen:    10,
			avgDocLen: 10,
			docCount:  4,
			docFreq:   map[string]int{"t": 1},
			// idf = ln(10/3); length factor = 1.
			// contribution = idf * (5*2.2)/(5 + 1.2*1) = idf * 11/6.2
			want: math.Log(10.0/3.0) * 11 / 6.2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.scorer.Score(tt.query, tt.tf, tt.docLen, tt.avgDocLen, tt.docCount, tt.docFreq)
			if !almostEqual(got, tt.want) {
				t.Errorf("Score() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBM25Score_EdgeCases(t *testing.T) {
	s := NewBM25Scorer(1.2, 0.75)

	// Zero/negative corpus stats must short-circuit to 0 (no div-by-zero).
	zeroCases := []struct {
		name      string
		docLen    float64
		avgDocLen float64
		docCount  int
	}{
		{"avgDocLen zero", 10, 0, 5},
		{"avgDocLen negative", 10, -1, 5},
		{"docCount zero", 10, 10, 0},
		{"docCount negative", 10, 10, -1},
		{"docLen zero", 0, 10, 5},
	}
	for _, tc := range zeroCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.Score([]string{"t"}, map[string]int{"t": 1}, tc.docLen, tc.avgDocLen, tc.docCount, map[string]int{"t": 1}); got != 0 {
				t.Errorf("Score() = %v, want 0", got)
			}
		})
	}

	// Empty query and empty tf both yield zero.
	if got := s.Score(nil, map[string]int{"t": 1}, 10, 10, 5, map[string]int{"t": 1}); got != 0 {
		t.Errorf("empty query Score() = %v, want 0", got)
	}
	if got := s.Score([]string{"t"}, nil, 10, 10, 5, map[string]int{"t": 1}); got != 0 {
		t.Errorf("nil tf Score() = %v, want 0", got)
	}
}

func TestBM25Score_IDFMonotonicity(t *testing.T) {
	// A rarer term (lower docFreq) must score higher than a common term.
	s := NewBM25Scorer(1.2, 0.75)
	rare := s.Score([]string{"rare"}, map[string]int{"rare": 1}, 10, 10, 100, map[string]int{"rare": 1})
	common := s.Score([]string{"common"}, map[string]int{"common": 1}, 10, 10, 100, map[string]int{"common": 99})
	if rare <= common {
		t.Errorf("rare term score %v must exceed common term score %v", rare, common)
	}
}

func TestBM25Score_Saturation(t *testing.T) {
	// BM25 saturates term-frequency contribution: doubling a high tf
	// must not double the score (k1 dampens the growth).
	s := NewBM25Scorer(1.2, 0.75)
	base := s.Score([]string{"t"}, map[string]int{"t": 10}, 10, 10, 4, map[string]int{"t": 1})
	doubled := s.Score([]string{"t"}, map[string]int{"t": 20}, 10, 10, 4, map[string]int{"t": 1})
	if doubled >= 2*base {
		t.Errorf("BM25 should saturate: doubled tf score %v >= 2*base %v", doubled, 2*base)
	}
}

func TestBM25ScoreWithIDF(t *testing.T) {
	tests := []struct {
		name      string
		query     []string
		tf        map[string]int
		idf       map[string]float64
		docLen    float64
		avgDocLen float64
		want      float64
	}{
		{
			name:      "uses precomputed idf",
			query:     []string{"t"},
			tf:        map[string]int{"t": 1},
			idf:       map[string]float64{"t": 2.0},
			docLen:    10,
			avgDocLen: 10,
			want:      2.0, // idf * (1*(2.2))/(1+1.2*1) = idf * 1
		},
		{
			name:      "missing idf entry is skipped",
			query:     []string{"t"},
			tf:        map[string]int{"t": 1},
			idf:       map[string]float64{},
			docLen:    10,
			avgDocLen: 10,
			want:      0,
		},
		{
			name:      "zero docLen short-circuits",
			query:     []string{"t"},
			tf:        map[string]int{"t": 1},
			idf:       map[string]float64{"t": 2.0},
			docLen:    0,
			avgDocLen: 10,
			want:      0,
		},
		{
			name:      "zero avgDocLen short-circuits",
			query:     []string{"t"},
			tf:        map[string]int{"t": 1},
			idf:       map[string]float64{"t": 2.0},
			docLen:    10,
			avgDocLen: 0,
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewBM25Scorer(1.2, 0.75)
			got := s.ScoreWithIDF(tt.query, tt.tf, tt.idf, tt.docLen, tt.avgDocLen)
			if !almostEqual(got, tt.want) {
				t.Errorf("ScoreWithIDF() = %v, want %v", got, tt.want)
			}
		})
	}
}
