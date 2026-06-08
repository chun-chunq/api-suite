package main
import ("context";"encoding/json";"fmt";"os";"time";"github.com/rs/zerolog";"github.com/insolvency-api/internal/scraper")
func main() {
	log := zerolog.New(zerolog.ConsoleWriter{Out:os.Stderr}).With().Logger()
	sc,_:=scraper.New(scraper.Options{Timeout:60*time.Second,BrowserBin:`C:\Program Files\Google\Chrome\Application\chrome.exe`,Logger:log})
	defer sc.Close()

	// Test 1: Nachname + Vorname
	fmt.Fprintln(os.Stderr,"=== Müller + Vorname Andreas, 30 Tage ===")
	ctx1,cancel1:=context.WithTimeout(context.Background(),60*time.Second); defer cancel1()
	r1,e1:=sc.Search(ctx1,scraper.SearchQuery{Name:"Müller",FirstName:"Andreas",DateFrom:time.Now().AddDate(0,0,-30),DateTo:time.Now()})
	if e1!=nil{fmt.Println("ERR:",e1)}else{
		out:=r1.Records; if len(out)>3{out=out[:3]}
		b,_:=json.MarshalIndent(out,"","  "); fmt.Println(string(b))
		fmt.Fprintf(os.Stderr,"Total: %d\n",r1.Totalfound)
	}

	// Test 2: Nachname + Stadt
	fmt.Fprintln(os.Stderr,"\n=== Schmidt + Stadt Berlin, 30 Tage ===")
	ctx2,cancel2:=context.WithTimeout(context.Background(),60*time.Second); defer cancel2()
	r2,e2:=sc.Search(ctx2,scraper.SearchQuery{Name:"Schmidt",City:"Berlin",DateFrom:time.Now().AddDate(0,0,-30),DateTo:time.Now()})
	if e2!=nil{fmt.Println("ERR:",e2)}else{
		out:=r2.Records; if len(out)>3{out=out[:3]}
		b,_:=json.MarshalIndent(out,"","  "); fmt.Println(string(b))
		fmt.Fprintf(os.Stderr,"Total: %d\n",r2.Totalfound)
	}
}