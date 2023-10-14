package main

import (
	"fmt"
	"sqlParser"
)

func main(){
	stmt, err := sqlParser.Unmarshal(`SELECT LOGDAT2 BIBRECNO, TITLE, AUTHOR, UPPER(REGEXP_REPLACE(ISBN, '[^0-9Xx]', '')) ISBN, SUBSTR(LOGDATE,1,4)||'-'||SUBSTR(LOGDATE,5,2)||'-'||SUBSTR(LOGDATE,7,2) LOGDATE, TO_CHAR((LOGSTAMP/1000 + 8*3600) / 86400 + TO_DATE('1970/01/01 00:00:00', 'YYYY/MM/DD HH24:MI:SS'),'HH24:MI:SS') LOGTIME FROM ILOG_CIR,BIBLIOS WHERE LOGDAT1=4741985 AND LOGTYPE=3031 AND LOGDAT2=BIBLIOS.RECNO ORDER BY LOGSTAMP DESC`)
	if err != nil {
		fmt.Println(err)
		return
	}
	/*var pars []sqlParser.Params
	pars = append(pars, sqlParser.Params{Name: ":LIB_INDEX"})
	pars = append(pars, sqlParser.Params{Name: ":TITLE"})
	stmt.DeleteParams(pars)*/
	//stmt.ExpandParams(sqlParser.Params{Name: ":CARDNO"}, 10)
	newSQL, err := sqlParser.Marshal(stmt)
	fmt.Println(newSQL)
}