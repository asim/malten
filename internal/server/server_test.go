package server
import ("net/http/httptest";"testing")
func TestShellAndRetiredRoutes(t *testing.T) {
 h:=New().Handler()
 for _, path:=range []string{"/","/app.js","/app.css","/manifest.webmanifest","/sw.js","/favicon.svg","/icon-192.png","/icon-512.png","/icon-maskable-512.png","/api/health"} {
 w:=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest("GET",path,nil));if w.Code!=200 {t.Errorf("%s: %d",path,w.Code)}
 }
 for _, path:=range []string{"/api/ask","/api/buses","/api/push/subscribe","/api/route","/api/interest","/api/tiles/1/1/1.png"} {
 w:=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest("GET",path,nil));if w.Code!=404 {t.Errorf("%s: %d",path,w.Code)}
 }
}
