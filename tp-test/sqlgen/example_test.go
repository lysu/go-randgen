package sqlgen

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestA(t *testing.T) {
	state := NewState()
	state.InjectTodoSQL("set @@tidb_enable_clustered_index=true")
	gen := NewGenerator(state)
	for i := 0; i < 200; i++ {
		fmt.Printf("%s;\n", gen())
	}
}

func TestCompareClusterVSNonCluster(t *testing.T) {
	deep, host, port := 20000, "127.0.0.1", "4000"
	var deepStr = os.Getenv("DEEP")
	if len(deepStr) > 0 {
		var err error
		deep, err = strconv.Atoi(deepStr)
		if err != nil {
			panic(err)
		}
	}
	if len(os.Getenv("HOST")) > 0 {
		host = os.Getenv("HOST")
	}
	if len(os.Getenv("PORT")) > 0 {
		port = os.Getenv("PORT")
	}
	for {
		doTest(host, port, deep)
	}
}

func doTest(host, port string, deep int) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("FAIL! %v", r)
		}
	}()
	gen := NewGenerator(NewState())

	c, closeC := testConn(host, port, "ctest")
	defer closeC()
	pIfe(c.ExecContext(context.Background(), "set @@tidb_enable_clustered_index=true"))

	nc, closeNc := testConn(host, port, "nctest")
	defer closeNc()
	pIfe(nc.ExecContext(context.Background(), "set @@tidb_enable_clustered_index=false"))

	for i := 0; i < deep; i++ {
		sql := gen()
		fmt.Println(sql)
		if strings.HasPrefix(strings.ToLower(sql), "select") {
			cRs, cErr := c.QueryContext(context.Background(), sql)
			ncRs, ncErr := nc.QueryContext(context.Background(), sql)
			if (cErr == nil && ncErr != nil) || (cErr != nil && ncErr == nil) || (cErr != nil && ncErr != nil && cErr.Error() != ncErr.Error()) {
				panic(fmt.Sprintf("sql:%s result not match %s vs %s", sql, cErr, ncErr))
			}
			if cRs != nil && ncRs != nil {
				func() {
					defer cRs.Close()
					defer ncRs.Close()
					for {
						nextC, nextNc := cRs.Next(), ncRs.Next()
						if (!nextC && nextNc) || (nextC && !nextNc) {
							panic(fmt.Sprintf("sql:%s result not match %t vs %t", sql, nextC, nextNc))
							break
						}
						if !nextNc && !nextC {
							break
						}
					}
				}()
			}
		} else {
			_, cErr := c.ExecContext(context.Background(), sql)
			_, ncErr := nc.ExecContext(context.Background(), sql)
			if (cErr == nil && ncErr != nil) || (cErr != nil && ncErr == nil) || (cErr != nil && ncErr != nil && cErr.Error() != ncErr.Error()) {
				panic(fmt.Sprintf("sql:%s result not match %s vs %s", sql, cErr, ncErr))
			}
		}
	}
}

func testConn(host, port, n string) (*sql.Conn, func()) {
	initDB, err := sql.Open("mysql", fmt.Sprintf(`root@tcp(%s:%s)/test`, host, port))
	if err != nil {
		panic(err)
	}
	defer initDB.Close()
	_, err = initDB.ExecContext(context.Background(), "drop database if exists "+n)
	if err != nil {
		panic(err)
	}
	_, err = initDB.ExecContext(context.Background(), "create database "+n)
	if err != nil {
		panic(err)
	}
	db, err := sql.Open("mysql", fmt.Sprintf(`root@tcp(%s:%s)/`+n, host, port))
	if err != nil {
		panic(err)
	}
	c, err := db.Conn(context.Background())
	if err != nil {
		panic(err)
	}
	return c, func() {
		c.Close()
		db.Close()
	}
}

func pIfe(vs ...interface{}) {
	if len(vs) == 0 {
		return
	}
	if e, isError := vs[len(vs)-1].(error); isError {
		panic(e)
	}
}
