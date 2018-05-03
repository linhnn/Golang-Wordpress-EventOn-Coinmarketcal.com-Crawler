package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/go-sql-driver/mysql"
)

const (
	DB_HOST    = ""
	DB_USER    = ""
	DB_PASS    = ""
	DB_NAME    = ""
	DB_CHARSET = "utf8"
)

var eventParam = map[string]string{
	"evcal_allday":                "yes",
	"evcal_event_color":           "206177",
	"evcal_event_color_n":         "1",
	"evcal_gmap_gen":              "no",
	"evcal_hide_locname":          "no",
	"evcal_lmlink_target":         "no",
	"evcal_name_over_img":         "no",
	"evcal_rep_freq":              "daily",
	"evcal_rep_gap":               "1",
	"evcal_rep_num":               "1",
	"evcal_repeat":                "no",
	"evo_access_control_location": "no",
	"evo_evcrd_field_org":         "no",
	"evo_exclude_ev":              "no",
	"evo_hide_endtime":            "no",
	"evo_repeat_wom":              "1",
	"evo_span_hidden_end":         "no",
	"evo_year_long":               "no",
	"evp_repeat_rb":               "dom",
	"evp_repeat_rb_wk":            "sing",
}

type EventService interface {
	AddEvent()
	UpdateEvent()
}

type Event struct {
	id          int
	subTitle    string
	name        string
	description string
	isHot       int
	isVerify    int
	vote        string
	eventDate   time.Time
	addedDate   time.Time
}

func main() {
	// db connection
	db, err := sql.Open("mysql", DB_USER+":"+DB_PASS+"@"+DB_HOST+"/"+DB_NAME+"?charset="+DB_CHARSET)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// crawl events
	c := make(chan string)
	var i int
	for i = 1; i <= 20; i++ {
		go CrawlEvent(db, i, c)
	}

	// get channel
	for i = 1; i <= 20*16; i++ {
		<-c
	}
}

func CrawlEvent(db *sql.DB, page int, c chan string) {
	// Request HTML page
	fmt.Println("https://coinmarketcal.com/?page=" + strconv.Itoa(page))
	res, err := http.Get("https://coinmarketcal.com/?page=" + strconv.Itoa(page))
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		log.Fatal("Error: %d %s", res.StatusCode, res.Status)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	var evt Event
	var id int

	doc.Find("article").Each(func(i int, s *goquery.Selection) {
		title := strings.TrimSpace(s.Find("h5").Eq(2).Text())            // title
		description := strings.TrimSpace(s.Find("p.description").Text()) // description
		description = strings.Replace(description, "\"", "", -1)

		vote := s.Find("span.votes").Text() // vote
		if vote != "" {
			voteReplace := strings.NewReplacer("(", "", ")", "", " votes", "", " vote", "")
			vote = voteReplace.Replace(vote)
		} else {
			vote = "0"
		}

		isVerify := s.Find("i.fa-badge-check").Length() // is verify
		isHot := s.Find("i.glyphicon-fire").Length()    // is hot

		coin := strings.Split(strings.TrimSpace(s.Find("h5").Eq(1).Text()), "(")
		coinName := coin[0]                               // coin name
		coinCode := strings.Replace(coin[1], ")", "", -1) // code name

		dateReplace := strings.NewReplacer("(Added", "", ")", "")
		date := strings.TrimSpace(dateReplace.Replace(s.Find("p.added-date").Text()))
		dateAdded, _ := time.Parse("02 January 2006", date) // added date

		dateReplace = strings.NewReplacer("By ", "", "(or earlier)", "")
		date = strings.TrimSpace(dateReplace.Replace(s.Find("h5 strong").Eq(0).Text()))
		dateEvent, _ := time.Parse("02 January 2006", date) // event date

		if isHot != 0 {
			err := db.QueryRow("SELECT id FROM wpll_posts WHERE post_title=? AND post_content=?", title, description).Scan(&id)
			switch {
			case err == sql.ErrNoRows: // event not exist
				fmt.Printf("Add %s %s\n", coinCode, title)
				evt = Event{
					subTitle:    coinName + "(" + coinCode + ")",
					name:        title,
					description: description,
					isHot:       isHot,
					isVerify:    isVerify,
					vote:        vote,
					eventDate:   dateEvent,
					addedDate:   dateAdded,
				}
				evt.AddEvent(db)
			case err != nil:
				log.Fatal(err)
			default: // event exists
				fmt.Printf("Update %s %s\n", coinCode, title)
				evt = Event{
					id:       id,
					vote:     vote,
					isVerify: isVerify,
				}
				evt.UpdateEvent(db)
			}
		}

		c <- title
	})

}

func (evt *Event) AddEvent(db *sql.DB) {
	replacer := strings.NewReplacer(" ", "-", "'", "", "\"", "")
	url := strings.ToLower(evt.name)
	url = replacer.Replace(url)

	// insert into wpll_posts
	result, err := db.Exec(
		"INSERT INTO wpll_posts(post_author, post_date, post_date_gmt, post_content, post_title, post_excerpt,  post_status, comment_status, ping_status, post_name, to_ping, pinged, post_modified, post_modified_gmt, post_content_filtered, post_parent, post_type) "+
			"VALUES(1, NOW(), NOW(), ?, ?, '', 'publish', 'open', 'closed', ?,'', '', NOW(), NOW(), '', 0, 'ajde_events')",
		evt.description,
		evt.name,
		url,
	)
	if err != nil {
		log.Fatal(err)
	}

	id, _ := result.LastInsertId() // get new ID

	// event date
	_, err = db.Exec(
		"INSERT INTO wpll_postmeta(post_id, meta_key, meta_value)"+
			"VALUES(?, 'evcal_srow', ?)",
		id,
		evt.eventDate.String(),
	)
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec(
		"INSERT INTO wpll_postmeta(post_id, meta_key, meta_value)"+
			"VALUES(?, 'evcal_erow', ?)",
		id,
		evt.eventDate.String(),
	)
	if err != nil {
		log.Fatal(err)
	}
	// event year date
	_, err = db.Exec(
		"INSERT INTO wpll_postmeta(post_id, meta_key, meta_value)"+
			"VALUES(?, 'event_year', ?)",
		id,
		evt.eventDate.Format("2006"),
	)
	if err != nil {
		log.Fatal(err)
	}
	// event month date
	_, err = db.Exec(
		"INSERT INTO wpll_postmeta(post_id, meta_key, meta_value)"+
			"VALUES(?, '_event_month', ?)",
		id,
		evt.eventDate.Format("01"),
	)
	if err != nil {
		log.Fatal(err)
	}
	// is feature
	_, err = db.Exec(
		"INSERT INTO wpll_postmeta(post_id, meta_key, meta_value)"+
			"VALUES(?, '_featured', 1)",
		id,
	)
	if err != nil {
		log.Fatal(err)
	}
	// event subtitle
	_, err = db.Exec(
		"INSERT INTO wpll_postmeta(post_id, meta_key, meta_value)"+
			"VALUES(?, 'evcal_subtitle', ?)",
		id,
		evt.subTitle,
	)
	if err != nil {
		log.Fatal(err)
	}
	// event vote
	_, err = db.Exec(
		"INSERT INTO wpll_postmeta(post_id, meta_key, meta_value)"+
			"VALUES(?, '_evcal_ec_f1a1_cus', ?)",
		id,
		evt.vote,
	)
	if err != nil {
		log.Fatal(err)
	}
	// event date added
	_, err = db.Exec(
		"INSERT INTO wpll_postmeta(post_id, meta_key, meta_value)"+
			"VALUES(?, '_evcal_ec_f3a1_cus', ?)",
		id,
		evt.addedDate.String(),
	)
	if err != nil {
		log.Fatal(err)
	}

	// is verify
	if evt.isVerify != 0 {
		_, err = db.Exec(
			"INSERT INTO wpll_postmeta(post_id, meta_key, meta_value)"+
				"VALUES(?, ?, 1)",
			id,
			"_evcal_ec_f2a1_cus",
		)
		if err != nil {
			log.Fatal(err)
		}
	}

	// other params
	for k, v := range eventParam {
		_, err = db.Exec(
			"INSERT INTO wpll_postmeta(post_id, meta_key, meta_value)"+
				"VALUES(?, ?, ?)",
			id,
			k,
			v,
		)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func (evt *Event) UpdateEvent(db *sql.DB) {
	// update event vote
	_, err := db.Exec(
		"UPDATE wpll_postmeta SET meta_value = ? "+
			"WHERE post_id = ? AND meta_key = '_evcal_ec_f1a1_cus' ",
		evt.vote,
		evt.id,
	)
	if err != nil {
		log.Fatal(err)
	}

	// is verify
	if evt.isVerify != 0 {
		_, err = db.Exec(
			"INSERT INTO wpll_postmeta(post_id, meta_key, meta_value)"+
				"VALUES(?, '_evcal_ec_f2a1_cus', 1) ON DUPLICATE KEY UPDATE meta_value=1",
			evt.id,
		)
		if err != nil {
			log.Fatal(err)
		}
	}
}
