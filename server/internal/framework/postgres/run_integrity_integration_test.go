//go:build integration

package postgres

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/framework/config"
)

// integrityDatabase 为约束测试创建独立真实 pgvector 库，清理范围仅限本测试数据库。
func integrityDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	admin, err := Open(t.Context(), cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	name := pgx.Identifier{"r1_" + strings.ToLower(rand.Text())}.Sanitize()
	if _, err := admin.Exec(t.Context(), "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+name); err != nil {
			t.Error(err)
		}
	})
	cfg.Postgres.Database = strings.Trim(name, `"`)
	db, err := Open(t.Context(), cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if _, err := db.Exec(t.Context(), "CREATE EXTENSION vector"); err != nil {
		t.Fatal(err)
	}
	return db
}

// seedIntegrity 创建合法关系及两位 owner，用于真实 SQL 的负向与级联断言。
func seedIntegrity(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	_, err := db.Exec(t.Context(), `
INSERT INTO users(id,workspace_id,email,password_hash,display_name) VALUES ('u1','default','u1@example.com',$1,'u1'),('u2','default','u2@example.com',$1,'u2'),('u3','default','u3@example.com',$1,'u3');
`, rand.Text())
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(t.Context(), `
INSERT INTO agent_definitions(id,workspace_id,owner_id,owner_type,name,visibility,status,created_by) VALUES ('a','default','u1','user','integrity','private','active','u1');
INSERT INTO agent_versions(id,workspace_id,agent_id,version_number,configuration,config_checksum,created_by) VALUES ('v','default','a',1,'{}','test','u1');
INSERT INTO conversations(id,workspace_id,user_id,agent_id) VALUES ('c1','default','u1','a'),('c2','default','u2','a');
INSERT INTO messages(id,conversation_id,role,content,status) VALUES ('m1','c1','user','约束测试','completed'),('m2','c2','user','约束测试','completed'),('mf','c1','assistant','回复','completed');
INSERT INTO runs(id,workspace_id,owner_user_id,conversation_id,agent_id,agent_version_id,root_run_id,user_message_id,final_message_id,provider,model,limits,status)
VALUES ('r1','default','u1','c1','a','v','r1','m1','mf','bailian','qwen3.8-flash','{}','completed');
INSERT INTO runs(id,workspace_id,owner_user_id,conversation_id,agent_id,agent_version_id,root_run_id,parent_run_id,user_message_id,provider,model,limits,status)
VALUES ('child','default','u1','c1','a','v','r1','r1','m1','bailian','qwen3.8-flash','{}','completed');
INSERT INTO tool_results(id,run_id,conversation_id,owner_user_id,step,call_index,call_id,tool_name,effect,status,idempotency_key)
VALUES ('tr','r1','c1','u1',1,0,'call','test','read','succeeded','r1:call');
INSERT INTO run_approvals(id,run_id,owner_user_id,call_id,tool_name,args_hash,display,status,expires_at)
VALUES ('approval','r1','u1','call','test','hash','{}','pending',now()+interval '1 hour');
INSERT INTO run_spans(id,run_id,kind,name,child_run_id,status,started_at) VALUES ('span','r1','subrun','child','child','ok',now());
UPDATE messages SET run_id='r1' WHERE id='mf';
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunIntegrityFreshAndUpgrade(t *testing.T) {
	for _, upgrade := range []bool{false, true} {
		name := "fresh"
		if upgrade {
			name = "upgrade_011"
		}
		t.Run(name, func(t *testing.T) {
			db := integrityDatabase(t)
			if upgrade {
				if _, err := db.Exec(t.Context(), `CREATE TABLE schema_migrations(version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
					t.Fatal(err)
				}
				names, err := migrationNames()
				if err != nil {
					t.Fatal(err)
				}
				for _, file := range names {
					if file >= "012" {
						break
					}
					body, err := migrationFiles.ReadFile("migrations/" + file)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := db.Exec(t.Context(), string(body)); err != nil {
						t.Fatal(err)
					}
					if _, err := db.Exec(t.Context(), `INSERT INTO schema_migrations(version) VALUES($1)`, file); err != nil {
						t.Fatal(err)
					}
				}
				seedIntegrity(t, db)
				// 旧库里可写入的非法值必须阻止升级，迁移失败不能删除或修改该行。
				if _, err := db.Exec(t.Context(), `UPDATE tool_results SET effect='not-an-effect'`); err != nil {
					t.Fatal(err)
				}
				if err := Migrate(t.Context(), db); err == nil {
					t.Fatal("非法存量未阻止迁移")
				}
				var preserved bool
				if err := db.QueryRow(t.Context(), `SELECT effect='not-an-effect' FROM tool_results WHERE id='tr'`).Scan(&preserved); err != nil || !preserved {
					t.Fatal("失败迁移改变了存量")
				}
				// 这里只修复本测试独立数据库内主动写入的非法样本。
				if _, err := db.Exec(t.Context(), `UPDATE tool_results SET effect='read'`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(t.Context(), `CREATE TABLE migration_before AS SELECT * FROM schema_migrations`); err != nil {
					t.Fatal(err)
				}
			}
			if err := Migrate(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			if err := CheckMigrations(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			if !upgrade {
				seedIntegrity(t, db)
			} else {
				var unchanged bool
				if err := db.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT * FROM migration_before EXCEPT SELECT * FROM schema_migrations)`).Scan(&unchanged); err != nil || !unchanged {
					t.Fatal("历史迁移记录变化")
				}
			}
			for _, tc := range []struct{ name, sql string }{
				{"insert_missing_root", `INSERT INTO runs(id,workspace_id,owner_user_id,conversation_id,agent_id,agent_version_id,root_run_id,parent_run_id,user_message_id,provider,model,limits,status) VALUES ('bad','default','u1','c1','a','v','run_missing','r1','m1','bailian','qwen3.8-flash','{}','completed')`},
				{"insert_cross_result", `INSERT INTO tool_results(id,run_id,conversation_id,owner_user_id,step,call_index,call_id,tool_name,effect,status,idempotency_key) VALUES ('bad','r1','c2','u1',1,1,'bad','test','read','pending','bad')`},
				{"insert_missing_result_owner", `INSERT INTO tool_results(id,run_id,conversation_id,owner_user_id,step,call_index,call_id,tool_name,effect,status,idempotency_key) VALUES ('bad','r1','c1','missing',1,1,'bad','test','read','pending','bad')`},
				{"insert_bad_effect", `INSERT INTO tool_results(id,run_id,conversation_id,owner_user_id,step,call_index,call_id,tool_name,effect,status,idempotency_key) VALUES ('bad','r1','c1','u1',1,1,'bad','test','not-an-effect','pending','bad')`},
				{"insert_missing_approval_owner", `INSERT INTO run_approvals(id,run_id,owner_user_id,call_id,tool_name,args_hash,display,status,expires_at) VALUES ('bad','r1','missing','bad','test','hash','{}','pending',now())`},
				{"missing_root", `UPDATE runs SET root_run_id='run_missing' WHERE id='child'`},
				{"top_not_self", `UPDATE runs SET root_run_id='child' WHERE id='r1'`},
				{"child_as_root", `UPDATE runs SET root_run_id='child' WHERE id='child'`},
				{"cross_owner", `UPDATE runs SET owner_user_id='u2' WHERE id='r1'`},
				{"cross_conversation", `UPDATE runs SET conversation_id='c2' WHERE id='r1'`},
				{"cross_message", `UPDATE runs SET user_message_id='m2' WHERE id='r1'`},
				{"cross_final", `UPDATE runs SET final_message_id='m2' WHERE id='r1'`},
				{"empty_key", `UPDATE runs SET idempotency_key='' WHERE id='r1'`},
				{"result_conversation", `UPDATE tool_results SET conversation_id='c2' WHERE id='tr'`},
				{"result_owner", `UPDATE tool_results SET owner_user_id='u2' WHERE id='tr'`},
				{"result_missing_owner", `UPDATE tool_results SET owner_user_id='missing' WHERE id='tr'`},
				{"effect", `UPDATE tool_results SET effect='not-an-effect' WHERE id='tr'`},
				{"result_empty_key", `UPDATE tool_results SET idempotency_key='' WHERE id='tr'`},
				{"approval_owner", `UPDATE run_approvals SET owner_user_id='u2' WHERE id='approval'`},
				{"approval_missing_owner", `UPDATE run_approvals SET owner_user_id='missing' WHERE id='approval'`},
				{"approval_decider", `UPDATE run_approvals SET decided_by='u2' WHERE id='approval'`},
				{"conversation_owner_update", `UPDATE conversations SET user_id='u3' WHERE id='c1'`},
			} {
				t.Run(tc.name, func(t *testing.T) {
					_, err := db.Exec(t.Context(), tc.sql)
					var pe *pgconn.PgError
					if !errors.As(err, &pe) || (pe.Code != "23503" && pe.Code != "23514") {
						t.Fatal("非法关系未被 FK/CHECK 拒绝")
					}
				})
			}
			// 合法状态变更及多条 NULL 幂等键不应被约束误伤。
			if _, err := db.Exec(t.Context(), `UPDATE run_approvals SET decided_by='u1',status='approved'; UPDATE tool_results SET effect='write'; UPDATE runs SET idempotency_key=NULL`); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct{ name, sql, check string }{
				{"final_message", `DELETE FROM messages WHERE id='mf'`, `SELECT count(*)=2 AND count(final_message_id)=0 FROM runs`},
				{"user_message", `DELETE FROM messages WHERE id='m1'`, `SELECT count(*)=0 FROM runs`},
				{"conversation", `DELETE FROM conversations WHERE id='c1'`, `SELECT count(*)=0 FROM runs`},
				{"parent", `DELETE FROM runs WHERE id='r1'`, `SELECT (SELECT count(*) FROM runs)=0 AND (SELECT count(*) FROM tool_results)=0 AND (SELECT count(*) FROM run_approvals)=0`},
				{"child", `DELETE FROM runs WHERE id='child'`, `SELECT child_run_id IS NULL FROM run_spans WHERE id='span'`},
			} {
				t.Run("cascade_"+tc.name, func(t *testing.T) {
					tx, err := db.Begin(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Rollback(context.Background())
					if _, err := tx.Exec(t.Context(), tc.sql); err != nil {
						t.Fatal(err)
					}
					var ok bool
					if err := tx.QueryRow(t.Context(), tc.check).Scan(&ok); err != nil || !ok {
						t.Fatal("级联结果错误")
					}
				})
			}
		})
	}
}
