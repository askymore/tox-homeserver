package store

import (
	"fmt"
	"gopp"
	"log"
	"os"
	"runtime"
	"time"
	"tox-homeserver/common"

	"github.com/go-xorm/xorm"
	// "github.com/hashicorp/go-uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/satori/go.uuid"
)

type Storage struct {
	dbh *xorm.Engine
}

func NewStorage() *Storage {
	this := &Storage{}
	var dsn string
	if runtime.GOOS == "android" {
		dsn = fmt.Sprintf("file:///data/data/io.dnesth.tofia/toxhs.sqlite")
	} else {
		dsn = fmt.Sprintf("toxhs.sqlite?cache=shared&mode=rwc")
	}
	dbh, err := xorm.NewEngine("sqlite3", dsn)
	gopp.ErrPrint(err)
	err = dbh.Ping()
	gopp.ErrPrint(err, dsn)
	this.dbh = dbh
	// this.SetWAL(true)

	logger := xorm.NewSimpleLogger2(os.Stdout, common.LogPrefix, 0)
	dbh.SetLogger(logger)
	dbh.ShowSQL(true)
	this.initTables()
	return this
}

// -wal when can delete this file? if lost, does it broken database?
// -shm when can delete this file? if lost, does it broken database?
func (this *Storage) SetWAL(enable bool) {
	_, err := this.dbh.Exec("PRAGMA journal_mode=WAL;")
	gopp.ErrPrint(err)
	_, err = this.dbh.Exec("PRAGMA locking_mode=EXCLUSIVE;")
	gopp.ErrPrint(err)
}

func (this *Storage) initTables() {
	tbls, err := this.dbh.DBMetas()
	gopp.ErrPrint(err, len(tbls))

	dmrecs := []interface{}{&Contact{}, &Message{}, &Device{}, &Idgen{}, &SyncInfo{}}
	for _, dmrec := range dmrecs {
		recval := fmt.Sprintf("%+v", dmrec)
		if ok, err := this.dbh.IsTableExist(dmrec); !ok && err == nil {
			err := this.dbh.CreateTables(dmrec)
			gopp.ErrPrint(err, recval)
		} else {
			err := this.dbh.Sync(dmrec)
			gopp.ErrPrint(err, recval)
		}

		if true {
			err = this.dbh.CreateUniques(dmrec)
			gopp.ErrPrint(err, recval)

			err = this.dbh.CreateIndexes(dmrec)
			gopp.ErrPrint(err, recval)

		}
	}
}

func (this *Storage) AddFriend(pubkey string, num uint32, name, stmsg string) (int64, error) {
	c := &Contact{}
	c.Pubkey = pubkey
	c.RtId = int(num)
	c.Name = name
	c.Stmsg = stmsg
	c.Status = 0
	c.IsFriend = 1
	return this.AddContact(c)
}

func (this *Storage) AddGroup(identify string, num uint32, title string) (int64, error) {
	c := &Contact{}
	c.IsGroup = 1
	c.Pubkey = identify
	c.RtId = int(num)
	c.Name = title
	return this.AddContact(c)
}

func (this *Storage) AddPeer(peerPubkey string, num uint32) (int64, error) {
	c := &Contact{}
	c.IsPeer = 1
	c.Pubkey = peerPubkey
	c.RtId = int(num)
	return this.AddContact(c)
}

func (this *Storage) AddContact(c *Contact) (int64, error) {
	nowt := time.Now().String()
	c.Created = nowt
	c.Updated = nowt

	id, err := this.dbh.InsertOne(c)
	gopp.ErrPrint(err, id)
	return id, err
}

func (this *Storage) AddFriendMessage(msg string, pubkey string) (int64, error) {
	c := &Contact{}
	c.Pubkey = pubkey
	exist, err := this.dbh.Get(c)
	gopp.ErrPrint(err, exist, pubkey)
	if err != nil {
		return 0, err
	}
	if !exist {
		return 0, xorm.ErrNotExist
	}

	m := &Message{}
	m.Content = msg
	m.ContactId = c.Id
	m.RoomId = c.Id // for friend, room id is contact id. contact is a room
	return this.AddMessage(m)
}

func (this *Storage) AddGroupMessage(msg string, mtype string, identify string, peerPubkey string) (int64, error) {
	c0 := &Contact{}
	c0.Pubkey = identify
	exist, err := this.dbh.Get(c0)
	gopp.ErrPrint(err, exist, identify)
	if err != nil {
		return 0, err
	}
	if !exist {
		return 0, xorm.ErrNotExist
	}

	c1 := &Contact{}
	c1.Pubkey = peerPubkey
	exist, err = this.dbh.Get(c1)
	gopp.ErrPrint(err, exist, identify)
	if err != nil {
		return 0, err
	}
	if !exist {
		return 0, xorm.ErrNotExist
	}

	m := &Message{}
	m.Content = msg
	m.ContactId = c1.Id
	m.RoomId = c0.Id
	return this.AddMessage(m)
}

func (this *Storage) AddMessage(m *Message) (int64, error) {
	nowt := time.Now().String()
	m.Updated = nowt
	// m.EventId <=0认为是server端，否则客户端
	if m.EventId <= 0 {
		m.Created = nowt
		m.EventId = this.NextId()
	}

	id, err := this.dbh.InsertOne(m)
	gopp.ErrPrint(err, id)
	return id, err
}

func (this *Storage) MaxEventId() (int64, error) {
	r := &Message{}
	exists, err := this.dbh.Desc("event_id").Limit(1).Get(r)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, xorm.ErrNotExist
	}
	return int64(r.EventId), nil
}

func (this *Storage) FindEventsByContactId(pubkey string, prev_batch int64) ([]Message, error) {
	c := this.GetContactByPubkey(pubkey)
	if c == nil {
		return nil, xorm.ErrNotExist
	}

	r := []Message{}
	err := this.dbh.Where("room_id = ? and event_id <= ?", c.Id, prev_batch).Desc("event_id").Limit(20).Find(&r)
	gopp.ErrPrint(err)
	return r, err
}

func (this *Storage) AddDevice() error {
	return this.AddDevice2(uuid.NewV4().String())
}

func (this *Storage) AddDevice2(name string) error {
	dv := Device{}
	dv.Uuid = name
	dv.Created = time.Now().String()
	dv.Updated = time.Now().String()

	id, err := this.dbh.InsertOne(&dv)
	gopp.ErrPrint(err, id)
	return err
}

func (this *Storage) DeviceEmpty() bool {
	dv := &Device{}
	empty, err := this.dbh.IsTableEmpty(dv)
	gopp.ErrPrint(err, empty)
	return err == nil && empty
}

func (this *Storage) GetDevice() *Device {
	dv := &Device{}
	_, err := this.dbh.Get(dv)
	gopp.ErrPrint(err)
	if err != nil {
		return nil
	}
	return dv
}

func (this *Storage) NextId() int64 {
	idv := &Idgen{}
	affected, err := this.dbh.InsertOne(idv)
	gopp.ErrPrint(err, affected)
	log.Println(affected, idv)
	return idv.Id
}

func (this *Storage) AddSyncInfo(ct_id int, next_batch int, prev_batch int) error {
	dv := SyncInfo{}
	dv.CtId = ct_id
	dv.NextBatch = next_batch
	dv.PrevBatch = prev_batch
	dv.Updated = time.Now().String()

	id, err := this.dbh.InsertOne(&dv)
	gopp.ErrPrint(err, id)
	return err
}

func (this *Storage) FindSyncInfoByCtId(ct_id int) ([]SyncInfo, error) {
	c := []SyncInfo{}
	err := this.dbh.Where("ct_id = ?", ct_id).Desc("next_batch").Find(c)
	gopp.ErrPrint(err, ct_id)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (this *Storage) UpdateSyncInfo(ct_id int, next_batch int, prev_batch int) error {
	c := &SyncInfo{}
	c.CtId = ct_id
	c.NextBatch = next_batch
	c.PrevBatch = prev_batch
	c.Updated = time.Now().String()

	_, err := this.dbh.Where("ct_id = ?", ct_id).Update(c)
	gopp.ErrPrint(err, ct_id)
	return err
}

func (this *Storage) DeleteSyncInfoByCtId(ct_id int) error {
	c := &SyncInfo{}
	c.CtId = ct_id
	_, err := this.dbh.Delete(c)
	gopp.ErrPrint(err, ct_id)
	return err
}

func init() {
	if false {
		xorm.NewEngine("", "")
		log.Println()
	}
}
