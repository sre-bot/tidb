// Copyright 2015 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package variable_test

import (
	"time"

	. "github.com/pingcap/check"
	"github.com/pingcap/parser"
	"github.com/pingcap/parser/auth"
	"github.com/pingcap/tidb/config"
	"github.com/pingcap/tidb/kv"
	"github.com/pingcap/tidb/sessionctx/stmtctx"
	"github.com/pingcap/tidb/sessionctx/variable"
	"github.com/pingcap/tidb/util/execdetails"
	"github.com/pingcap/tidb/util/mock"
)

var _ = SerialSuites(&testSessionSuite{})

type testSessionSuite struct {
}

func (*testSessionSuite) TestSession(c *C) {
	ctx := mock.NewContext()

	ss := ctx.GetSessionVars().StmtCtx
	c.Assert(ss, NotNil)

	// For AffectedRows
	ss.AddAffectedRows(1)
	c.Assert(ss.AffectedRows(), Equals, uint64(1))
	ss.AddAffectedRows(1)
	c.Assert(ss.AffectedRows(), Equals, uint64(2))

	// For RecordRows
	ss.AddRecordRows(1)
	c.Assert(ss.RecordRows(), Equals, uint64(1))
	ss.AddRecordRows(1)
	c.Assert(ss.RecordRows(), Equals, uint64(2))

	// For FoundRows
	ss.AddFoundRows(1)
	c.Assert(ss.FoundRows(), Equals, uint64(1))
	ss.AddFoundRows(1)
	c.Assert(ss.FoundRows(), Equals, uint64(2))

	// For UpdatedRows
	ss.AddUpdatedRows(1)
	c.Assert(ss.UpdatedRows(), Equals, uint64(1))
	ss.AddUpdatedRows(1)
	c.Assert(ss.UpdatedRows(), Equals, uint64(2))

	// For TouchedRows
	ss.AddTouchedRows(1)
	c.Assert(ss.TouchedRows(), Equals, uint64(1))
	ss.AddTouchedRows(1)
	c.Assert(ss.TouchedRows(), Equals, uint64(2))

	// For CopiedRows
	ss.AddCopiedRows(1)
	c.Assert(ss.CopiedRows(), Equals, uint64(1))
	ss.AddCopiedRows(1)
	c.Assert(ss.CopiedRows(), Equals, uint64(2))

	// For last insert id
	ctx.GetSessionVars().SetLastInsertID(1)
	c.Assert(ctx.GetSessionVars().StmtCtx.LastInsertID, Equals, uint64(1))

	ss.ResetForRetry()
	c.Assert(ss.AffectedRows(), Equals, uint64(0))
	c.Assert(ss.FoundRows(), Equals, uint64(0))
	c.Assert(ss.UpdatedRows(), Equals, uint64(0))
	c.Assert(ss.RecordRows(), Equals, uint64(0))
	c.Assert(ss.TouchedRows(), Equals, uint64(0))
	c.Assert(ss.CopiedRows(), Equals, uint64(0))
	c.Assert(ss.WarningCount(), Equals, uint16(0))
}

func (*testSessionSuite) TestSlowLogFormat(c *C) {
	ctx := mock.NewContext()

	seVar := ctx.GetSessionVars()
	c.Assert(seVar, NotNil)

	seVar.User = &auth.UserIdentity{Username: "root", Hostname: "192.168.0.1"}
	seVar.ConnectionID = 1
	seVar.CurrentDB = "test"
	seVar.InRestrictedSQL = true
	txnTS := uint64(406649736972468225)
	costTime := time.Second
	execDetail := execdetails.ExecDetails{
		ProcessTime:   time.Second * time.Duration(2),
		WaitTime:      time.Minute,
		BackoffTime:   time.Millisecond,
		RequestCount:  2,
		TotalKeys:     10000,
		ProcessedKeys: 20001,
	}
	statsInfos := make(map[string]uint64)
	statsInfos["t1"] = 0
	copTasks := &stmtctx.CopTasksDetails{
		NumCopTasks:       10,
		AvgProcessTime:    time.Second,
		P90ProcessTime:    time.Second * 2,
		MaxProcessAddress: "10.6.131.78",
		MaxProcessTime:    time.Second * 3,
		AvgWaitTime:       time.Millisecond * 10,
		P90WaitTime:       time.Millisecond * 20,
		MaxWaitTime:       time.Millisecond * 30,
		MaxWaitAddress:    "10.6.131.79",
	}
	var memMax int64 = 2333
	resultString := `# Txn_start_ts: 406649736972468225
# User: root@192.168.0.1
# Conn_ID: 1
# Query_time: 1
# Parse_time: 0.00000001
# Compile_time: 0.00000001
# Process_time: 2 Wait_time: 60 Backoff_time: 0.001 Request_count: 2 Total_keys: 10000 Process_keys: 20001
# DB: test
# Index_names: [t1:a,t2:b]
# Is_internal: true
# Digest: 42a1c8aae6f133e934d4bf0147491709a8812ea05ff8819ec522780fe657b772
# Stats: t1:pseudo
# Num_cop_tasks: 10
# Cop_proc_avg: 1 Cop_proc_p90: 2 Cop_proc_max: 3 Cop_proc_addr: 10.6.131.78
# Cop_wait_avg: 0.01 Cop_wait_p90: 0.02 Cop_wait_max: 0.03 Cop_wait_addr: 10.6.131.79
# Mem_max: 2333
# Prepared: true
# Has_more_results: true
# Succ: true
select * from t;`
	sql := "select * from t"
	_, digest := parser.NormalizeDigest(sql)
	logString := seVar.SlowLogFormat(&variable.SlowQueryLogItems{
		TxnTS:          txnTS,
		SQL:            sql,
		Digest:         digest,
		TimeTotal:      costTime,
		TimeParse:      time.Duration(10),
		TimeCompile:    time.Duration(10),
		IndexNames:     "[t1:a,t2:b]",
		StatsInfos:     statsInfos,
		CopTasks:       copTasks,
		ExecDetail:     execDetail,
		MemMax:         memMax,
		Prepared:       true,
		HasMoreResults: true,
		Succ:           true,
	})
	c.Assert(logString, Equals, resultString)
}

func (*testSessionSuite) TestIsolationRead(c *C) {
	originIsolationEngines := config.GetGlobalConfig().IsolationRead.Engines
	defer func() {
		config.GetGlobalConfig().IsolationRead.Engines = originIsolationEngines
	}()
	config.GetGlobalConfig().IsolationRead.Engines = []string{"tiflash", "tidb"}
	sessVars := variable.NewSessionVars()
	_, ok := sessVars.IsolationReadEngines[kv.TiDB]
	c.Assert(ok, Equals, true)
	_, ok = sessVars.IsolationReadEngines[kv.TiKV]
	c.Assert(ok, Equals, false)
	_, ok = sessVars.IsolationReadEngines[kv.TiFlash]
	c.Assert(ok, Equals, true)
}
