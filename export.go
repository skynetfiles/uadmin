package uadmin

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/tealeg/xlsx"
)

type adminPager interface {
	AdminPage(string, bool, int, int, interface{}, interface{}, ...interface{}) error
}

func getFilter(r *http.Request, session *Session) (interface{}, []interface{}) {
	queryList := []string{}
	args := []interface{}{}
	for k, v := range r.URL.Query() {

		if k == "m" || k == "o" || k == "p" {
			continue
		}

		if k == "q" {
			continue
		}

		user := session.User

		if len(v) > 0 {
			v[0] = strings.Replace(v[0], "{username}", user.Username, -1)
			v[0] = strings.Replace(v[0], "{me}", user.Username, -1)
			v[0] = strings.Replace(v[0], "{userid}", fmt.Sprint(user.ID), -1)
		}

		queryParts := strings.Split(k, "__")
		query := "`" + queryParts[0] + "`"
		if len(queryParts) > 1 {
			if queryParts[1] == "lt" {
				// Less than
				query += " < ?"
			}
			if queryParts[1] == "lte" {
				// Less than or equal to
				query += " <= ?"
			}
			if queryParts[1] == "gt" {
				// Greater than
				query += " > ?"
			}
			if queryParts[1] == "gte" {
				// Greater than or equal to
				query += " >= ?"
			}
			if queryParts[1] == "in" {
				// Greater than or equal to
				query += " IN (?)"
			}
			if queryParts[1] == "contains" {
				// Greater than or equal to
				query += " LIKE ?"
			}
		} else {
			query += " = ?"
		}
		if len(queryParts) > 1 && queryParts[1] == "in" {
			args = append(args, strings.Split(v[0], ","))
		} else if len(queryParts) > 1 && queryParts[1] == "contains" {
			args = append(args, "%"+v[0]+"%")
		} else {
			args = append(args, v[0])
		}
		queryList = append(queryList, query)
	}
	return strings.Join(queryList, " AND "), args
}

// exportHandler handles http request for exporting data
func exportHandler(w http.ResponseWriter, r *http.Request, session *Session) {
	//http://hostname/admin/export/?m=orders&date__gte=2016-02-01&date__lte=2016-03-01
	var err error

	// TODO: Call ListSchemaModifier of the schema and use the modified one

	modelName := r.URL.Query().Get("m")
	schema, ok := getSchema(modelName)
	if !ok {
		page404Handler(w, r, session)
		return
	}

	a, ok := NewModelArray(modelName, false)
	if !ok {
		page404Handler(w, r, session)
		return
	}
	m, _ := NewModel(modelName, false)

	query, args := getFilter(r, session)

	ap, ok := m.Interface().(adminPager)

	if ok {
		err = ap.AdminPage("id", true, 0, -1, a.Addr().Interface(), query, args...)
	} else {
		err = AdminPage("id", true, 0, -1, a.Addr().Interface(), query, args...)
	}

	if err != nil {
		page404Handler(w, r, session)
		return
	}

	var file *xlsx.File
	var sheet *xlsx.Sheet
	var row *xlsx.Row
	var cell *xlsx.Cell

	file = xlsx.NewFile()
	sheet, err = file.AddSheet("Sheet1")
	if err != nil {
		page404Handler(w, r, session)
		Trail(ERROR, "Error in exportHandler, unable to add sheet. %s", err)
		return
	}

	t := reflect.TypeOf(m.Interface())

	// Header
	row = sheet.AddRow()
	headerStyle := xlsx.NewStyle()
	headerStyle.Font.Bold = true
	headerStyle.Font.Size = 10
	headerStyle.Font.Name = "Arial"
	headerStyle.ApplyFont = true
	for i := 0; i < m.NumField(); i++ {
		if !schema.FieldByName(t.Field(i).Name).ListDisplay || m.Field(i).Type().Name() == "Model" || (m.Field(i).Type().Kind() == reflect.Uint && strings.HasSuffix(t.Field(i).Name, "ID")) {
			continue
		}
		cell = row.AddCell()
		cell.SetStyle(headerStyle)
		cell.Value = getDisplayName(t.Field(i).Name)
	}

	// Body (Data)
	now := time.Now()
	for i := 0; i < a.Len(); i++ {
		preloaded := false
		row = sheet.AddRow()
		colIndex := -1
		for c := 0; c < m.NumField(); c++ {
			if !schema.FieldByName(t.Field(c).Name).ListDisplay || m.Field(c).Type().Name() == "Model" || (m.Field(c).Type().Kind() == reflect.Uint && strings.HasSuffix(t.Field(c).Name, "ID")) {
				continue
			}
			colIndex++

			// Add a new cell
			cell = row.AddCell()

			// Get data based on data type
			if t.Field(c).Type.Kind() == reflect.Float64 {
				cell.SetFloat(a.Index(i).Field(c).Float())
				//cell.Value = fmt.Sprintf("%.2f", a.Index(i).Field(c).Float())
			} else if t.Field(c).Type == reflect.TypeOf(&now) {
				dt, ok := a.Index(i).Field(c).Interface().(*time.Time)
				if ok && dt != nil {
					//cell.Value = dt.Format("2006-01-02 15:04:05")
					cell.SetDateTime(*dt)
					cell.NumFmt = "YYYY-MM-DD HH:MM AM/PM"
					sheet.Col(colIndex).Width = 13.4
				}
			} else if t.Field(c).Type == reflect.TypeOf(now) {
				dt, ok := a.Index(i).Field(c).Interface().(time.Time)
				if ok {
					//cell.Value = dt.Format("2006-01-02 15:04:05")
					cell.SetDateTime(dt)
					cell.NumFmt = "YYYY-MM-DD HH:MM AM/PM"
					sheet.Col(colIndex).Width = 13.4
				}
			} else if t.Field(c).Type.Kind() == reflect.Struct || (t.Field(c).Type.Kind() == reflect.Ptr && t.Field(c).Type.Elem().Kind() == reflect.Struct) {
				if !preloaded {
					Preload(a.Index(i).Addr().Interface())
				}
				cell.Value = GetString(a.Index(i).Field(c).Interface())
			} else if t.Field(c).Type.Kind() == reflect.Int {
				if t.Field(c).Type == reflect.TypeOf(0) {
					cell.SetInt(int(a.Index(i).Field(c).Int()))
					// cell.Value = fmt.Sprint(a.Index(i).Field(c).Int())
				} else {
					value := a.Index(i).Field(c).Int()
					for mIndex := 0; mIndex < t.Field(c).Type.NumMethod(); mIndex++ {
						rValue := a.Index(i).Field(c).Method(mIndex).Call([]reflect.Value{})[0].Int()
						if rValue == value {
							cell.Value = getDisplayName(t.Field(c).Type.Method(mIndex).Name)
							break
						}
					}
				}
			} else if t.Field(c).Type.Kind() == reflect.Bool {
				cell.SetBool(a.Index(i).Field(c).Bool())
				// cell.Value = fmt.Sprint(a.Index(i).Field(c).Bool())
			} else {
				cell.Value = fmt.Sprint(a.Index(i).Field(c).Interface())
			}
		}
	}
	exportRoot := "./media/export/"
	if _, err = os.Stat(exportRoot); os.IsNotExist(err) {
		os.MkdirAll(exportRoot, 0700)
		os.Create(exportRoot + "index.html")
	}

	fileName := GenerateBase64(24)
	err = file.Save("./media/export/" + fileName + ".xlsx")
	if err != nil {
		fmt.Printf(err.Error())
	}
	http.Redirect(w, r, "/media/export/"+fileName+".xlsx", 303)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")

	// Check if the file exists

}
