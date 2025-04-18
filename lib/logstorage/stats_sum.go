package logstorage

import (
	"fmt"
	"math"
	"strconv"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/bytesutil"
)

type statsSum struct {
	fields []string
}

func (ss *statsSum) String() string {
	return "sum(" + statsFuncFieldsToString(ss.fields) + ")"
}

func (ss *statsSum) updateNeededFields(neededFields fieldsSet) {
	updateNeededFieldsForStatsFunc(neededFields, ss.fields)
}

func (ss *statsSum) newStatsProcessor(a *chunkedAllocator) statsProcessor {
	ssp := a.newStatsSumProcessor()
	ssp.sum = nan
	return ssp
}

type statsSumProcessor struct {
	sum float64
}

func (ssp *statsSumProcessor) updateStatsForAllRows(sf statsFunc, br *blockResult) int {
	ss := sf.(*statsSum)
	fields := ss.fields
	if len(fields) == 0 {
		// Sum all the columns
		for _, c := range br.getColumns() {
			ssp.updateStateForColumn(br, c)
		}
	} else {
		// Sum the requested columns
		for _, field := range fields {
			c := br.getColumnByName(field)
			ssp.updateStateForColumn(br, c)
		}
	}
	return 0
}

func (ssp *statsSumProcessor) updateStatsForRow(sf statsFunc, br *blockResult, rowIdx int) int {
	ss := sf.(*statsSum)
	fields := ss.fields
	if len(fields) == 0 {
		// Sum all the fields for the given row
		for _, c := range br.getColumns() {
			f, ok := c.getFloatValueAtRow(br, rowIdx)
			if ok {
				ssp.updateState(f)
			}
		}
	} else {
		// Sum only the given fields for the given row
		for _, field := range fields {
			c := br.getColumnByName(field)
			f, ok := c.getFloatValueAtRow(br, rowIdx)
			if ok {
				ssp.updateState(f)
			}
		}
	}
	return 0
}

func (ssp *statsSumProcessor) updateStateForColumn(br *blockResult, c *blockResultColumn) {
	f, count := c.sumValues(br)
	if count > 0 {
		ssp.updateState(f)
	}
}

func (ssp *statsSumProcessor) updateState(f float64) {
	if math.IsNaN(ssp.sum) {
		ssp.sum = f
	} else {
		ssp.sum += f
	}
}

func (ssp *statsSumProcessor) mergeState(_ *chunkedAllocator, _ statsFunc, sfp statsProcessor) {
	src := sfp.(*statsSumProcessor)
	if !math.IsNaN(src.sum) {
		ssp.updateState(src.sum)
	}
}

func (ssp *statsSumProcessor) exportState(dst []byte, _ <-chan struct{}) []byte {
	return marshalFloat64(dst, ssp.sum)
}

func (ssp *statsSumProcessor) importState(src []byte, _ <-chan struct{}) (int, error) {
	if len(src) != 8 {
		return 0, fmt.Errorf("unexpected state length; got %d bytes; want 8 bytes", len(src))
	}
	ssp.sum = unmarshalFloat64(bytesutil.ToUnsafeString(src))
	return 0, nil
}

func (ssp *statsSumProcessor) finalizeStats(_ statsFunc, dst []byte, _ <-chan struct{}) []byte {
	return strconv.AppendFloat(dst, ssp.sum, 'f', -1, 64)
}

func parseStatsSum(lex *lexer) (*statsSum, error) {
	fields, err := parseStatsFuncFields(lex, "sum")
	if err != nil {
		return nil, err
	}
	ss := &statsSum{
		fields: fields,
	}
	return ss, nil
}
