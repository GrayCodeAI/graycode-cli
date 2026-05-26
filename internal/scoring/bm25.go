package scoring

import "math"

// DefaultBM25K1 is the default term frequency saturation parameter.
const DefaultBM25K1 = 1.2

// DefaultBM25B is the default document length normalization parameter.
const DefaultBM25B = 0.75

// BM25Scorer computes BM25 scores with configurable parameters.
type BM25Scorer struct {
	K1 float64
	B  float64
}

// NewBM25Scorer creates a BM25Scorer with the given parameters.
// If k1 or b is <= 0, the default values (k1=1.2, b=0.75) are used.
func NewBM25Scorer(k1, b float64) *BM25Scorer {
	if k1 <= 0 {
		k1 = DefaultBM25K1
	}
	if b <= 0 {
		b = DefaultBM25B
	}
	return &BM25Scorer{K1: k1, B: b}
}

// Score computes the BM25 score for a document given query terms.
// It computes IDF on the fly from docFreq and docCount.
//
// Parameters:
//   - queryTerms: tokenized query terms
//   - docTermFreq: term frequency map for the document being scored
//   - docLen: the document's length (total term count)
//   - avgDocLen: average document length across the corpus
//   - docCount: total number of documents in the corpus (N)
//   - docFreq: number of documents containing each term (n(t))
//
// Uses the standard BM25 formula:
//
//	sum over t in query of: IDF(t) * (tf * (k1+1)) / (tf + k1 * (1 - b + b * dl/avgdl))
//
// with IDF(t) = log((N - n(t) + 0.5) / (n(t) + 0.5) + 1)
func (s *BM25Scorer) Score(queryTerms []string, docTermFreq map[string]int, docLen float64, avgDocLen float64, docCount int, docFreq map[string]int) float64 {
	if avgDocLen <= 0 || docCount <= 0 || docLen == 0 {
		return 0
	}

	n := float64(docCount)

	var score float64
	for _, term := range queryTerms {
		tf := float64(docTermFreq[term])
		if tf == 0 {
			continue
		}

		df := float64(docFreq[term])
		idf := math.Log((n-df+0.5)/(df+0.5) + 1.0)

		numerator := tf * (s.K1 + 1)
		denominator := tf + s.K1*(1-s.B+s.B*(docLen/avgDocLen))
		score += idf * (numerator / denominator)
	}

	return score
}

// ScoreWithIDF is a convenience method for callers that already have precomputed
// IDF values and know the document length directly.
//
// Parameters:
//   - queryTerms: tokenized query terms
//   - docTermFreq: term frequency map for the document
//   - idf: precomputed IDF value for each term
//   - docLen: the document's length (total term count)
//   - avgDocLen: average document length across the corpus
func (s *BM25Scorer) ScoreWithIDF(queryTerms []string, docTermFreq map[string]int, idf map[string]float64, docLen float64, avgDocLen float64) float64 {
	if docLen == 0 || avgDocLen == 0 {
		return 0
	}

	var score float64
	for _, term := range queryTerms {
		tf := float64(docTermFreq[term])
		if tf == 0 {
			continue
		}

		idfVal, exists := idf[term]
		if !exists {
			continue
		}

		numerator := tf * (s.K1 + 1)
		denominator := tf + s.K1*(1-s.B+s.B*(docLen/avgDocLen))
		score += idfVal * (numerator / denominator)
	}

	return score
}
