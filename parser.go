package sqlParser

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Value SQL的值，它可以是子查询、函数、CASE WHEN表达式、字符串、数字（应当包括加减乘除等运算）、字段TableField（即不被括号括起来的，包含了像SYSDATE这样的关键词）、参数、被双竖线连接的值组合；它可以出现在：查询的字段、条件语句的左右值、新增/更新语句的值
type Value struct {
	Value interface{} //它可以是Function、CaseWhen、String、Number、TableField、Params、ConcatValue
}

//TableField 它可以是字段(可能被双引号括起)、参数；它可以出现在新增、更新的字段上。
//type TableField struct {
//	Field	interface{}		//它可以是字符串（包含了双引号的）、Params
//}

// Equation 等式，它可以是 “左值 符号（>、>=、<、<=、=） 右值”，“Between 值 and 值”，“值 IS (NOT)? NULL”，“值 (NOT)? LIKE 值”，“值 (NOT)? IN(值...)”，“值 (NOT)? EXIST(值...)”。它们之间需使用AND/OR连接
type Equation struct {
	Equation  interface{} //它可以是EquationNorm、EquationOther、EquationBetween, EquationList
	Connector string      //连接符只能是AND、OR两种
}

// EquationNorm 标准等式，即左值 符号 右值
type EquationNorm struct {
	Left     Value
	Right    Value
	Operator string
}

type EquationBetween struct {
	Left  Value
	Right Value
	Field Value
}

// EquationOther 其他等式
type EquationOther struct {
	Left     Value
	Operator string  //它可以是IS (NOT)?   (NOT)? (LIKE)|(IN)|(EXIST)
	Right    []Value //如果是NULL的，直接就是字符串NULL，其他的则需要括号表示
}

// EquationList 多个条件可以被小括号括起
type EquationList struct {
	Equation  []Equation
	Connector string
}

// Function 函数：函数必须是一个函数名，外加参数组成的，参数里面的值可以是0个或多个
type Function struct {
	Name   string
	Params []Value
}

/*
CaseWhen case when表达式：它有两种表达方式
1。case 值 when 值 then 值 else 值 end;
2。case when 条件 then 值 else 值 end;
不管是哪种表达式，实际上最终都指向了第二种，第一种翻译成第二种就是
case when 值=值 then 值 else 值 end;
*/
type CaseWhen struct {
	When []CaseWhenItem
	Else Value
}

// CaseWhenItem case when表达式的单个条件项
type CaseWhenItem struct {
	Equation EquationList
	Value    Value
}

//String 被单引号括起的部分
//type String struct {
//	Value		string
//}

// Number 数字，它可以是单数字，也可以是值+-*/值
type Number struct {
	Number []NumberItem
}

// NumberItem 数字的单个选项
type NumberItem struct {
	Value    Value
	Operator string
}

// Params 参数，即冒号开头的参数占位符
type Params struct {
	Name string
}

//ConcatValue 竖线连接的值
//type ConcatValue struct {
//	Value		[]Value
//}

type OrderBy struct {
	Value     []Value
	Collation string
}

type Statement struct {
	Ast interface{}
}

// Select 一个完整的SQL语句应该可由多个单查询组合而成，加上像UNION等关键词进行合并
type Select struct {
	Select []SelectItem
}

type SelectItem struct {
	Field     []SelectField
	Table     []SelectTable
	Where     EquationList
	Group     []Value
	Having    EquationList
	Order     interface{} //它可以是[]Value(Order By)、Function(Order Decode)
	Aggregate string      //集合关键词：union、union all、minus、intersect
}

type SelectField struct {
	Field Value
	Alias string
}

type SelectTable struct {
	Table   interface{}  //它可以是子查询，也可以是字符串
	Alias   string       //别名
	JoinKey string       //如果这张表是join前面的表，则会有关键词，它可以是JOIN、LEFT JOIN、RIGHT JOIN、INNER JOIN
	JoinOn  EquationList //一个条件列，它可以被括号括起来
}

type Placeholder struct {
	Value string
	Name  string //所有占位符的名称都应该是$序号，例$000001
}

type Insert struct {
	Table  string
	Field  []string
	Values interface{} //它可以时[]Value，或者Select。
}

type UpdateValueItem struct {
	Field string
	Value Value
}

type Update struct {
	Table string
	Value []UpdateValueItem
	Where EquationList
}

// Delete 删除数据的时候，可能会有FROM关键词，为了兼容以前的ORACLE，生成SQL的时候带上FROM
type Delete struct {
	Table string
	Where EquationList
}

// removeExtraSpaces 清除多余的空格
func removeExtraSpaces(s string) string {
	//将多个空格、制表符、换行符替换成一个空格
	reg, _ := regexp.Compile("[ \t\n]+")
	str := reg.ReplaceAllString(s, " ")
	return str
}

// placeholderByString 将一段SQL中可以替换成占位符的字符串替换成占位符
func placeholderByString(s string, placeholder *[]Placeholder, placeholderPos *int) (string, error) {
	s = strings.TrimSpace(s)
	//替换单引号括起的内容
	s = replacePlaceholder(s, `'(?:[^']|'')*'`, placeholder, placeholderPos)
	//替换双引号
	s = replacePlaceholder(s, "\".*?\"", placeholder, placeholderPos)
	//替换反单引号
	s = replacePlaceholder(s, "`.*?`", placeholder, placeholderPos)
	//替换参数
	s = replacePlaceholder(s, `:\w*`, placeholder, placeholderPos)
	//转大写
	s = strings.ToUpper(s)
	//左括号左侧要空格，右侧去空格
	s = strings.ReplaceAll(s, "(", " (")
	s = strings.ReplaceAll(s, "( ", "(")
	//右括号左侧去空格，右侧要空格
	s = strings.ReplaceAll(s, ")", ") ")
	s = strings.ReplaceAll(s, " )", ")")
	//删除多余的空格
	s = removeExtraSpaces(s)
	//从里到外，把括号的内容替换成占位符
	var err error
	s, err = replaceParenthesis(s, placeholder, placeholderPos)
	return s, err
}

// replacePlaceholder 用正则表达式替换占位符
func replacePlaceholder(s string, regStr string, placeholder *[]Placeholder, placeholderPos *int) string {
	re := regexp.MustCompile(regStr)
	matches := re.FindAllString(s, -1)
	strs := re.Split(s, -1)
	var ret string
	for i := 0; i < len(strs); i++ {
		ret += strs[i]
		if len(matches) > i {
			name := fmt.Sprintf("$%06d", *placeholderPos)
			*placeholderPos++
			*placeholder = append(*placeholder, Placeholder{Name: name, Value: matches[i]})
			ret += name
		}
	}
	return ret
}

// replaceParenthesis 把有小括号的替换成占位符，要求从里到外所有的括号依次替换
func replaceParenthesis(s string, placeholder *[]Placeholder, placeholderPos *int) (string, error) {
	//利用栈的原理去处理
	var leftStack []int
	for index, c := range s {
		if c == '(' {
			leftStack = append(leftStack, index)
		} else if c == ')' {
			//需要替换这一段的数据
			if len(leftStack) > 0 {
				name := fmt.Sprintf("$%06d", *placeholderPos)
				*placeholderPos++
				*placeholder = append(*placeholder, Placeholder{Name: name, Value: s[leftStack[len(leftStack)-1] : index+1]})
				s = s[:leftStack[len(leftStack)-1]] + name + s[index+1:]
				return replaceParenthesis(s, placeholder, placeholderPos)
			}
			return "", errors.New("缺失左括号")
		}
	}
	if len(leftStack) > 0 {
		return "", errors.New("缺失右括号")
	}
	return s, nil
}

// getPlaceholder 传入一个字符串，返回被占位符替代还原后的字符串和占位符列表
func getPlaceholder(s string, placeholder *[]Placeholder, placeholderPos *int) (retStr string, retPlace []Placeholder, err error) {
	//按$取出所有参数
	re := regexp.MustCompile(`\$[0-9]+`)
	matches := re.FindAllString(s, -1)
	for _, item := range matches {
		idx, err := strconv.Atoi(item[1:])
		if err != nil {
			return "", nil, errors.New("获取占位符失败")
		}
		s = strings.ReplaceAll(s, item, (*placeholder)[idx].Value)
		retPlace = append(retPlace, Placeholder{Name: item, Value: (*placeholder)[idx].Value})
	}
	return s, retPlace, err
}

// getSqlType 获取SQL的语法类型
func getSqlType(s string, placeholder *[]Placeholder, placeholderPos *int) (string, error) {
	strs := strings.Split(s, " ")
	if strs[0][0] == '$' {
		//占位符，说明多有是被括号括起的
		newStr, _, err := getPlaceholder(s, placeholder, placeholderPos)
		if err != nil {
			return "", err
		}
		return getSqlType(newStr, placeholder, placeholderPos)
	}
	return strs[0], nil
}

// trimLR 同时满足左右两边都有的情况下才会去掉
func trimLR(s, l, r string) string {
	if s == "" {
		return s
	}
	nLeft := strings.Index(s, l)
	nRight := strings.LastIndex(s, r)
	if nLeft == -1 || nRight == -1 {
		return s
	}
	if nLeft == 0 && nRight == len(s)-len(r) {
		//可以去掉
		s = s[:nRight]
		s = s[nLeft+len(l):]
	}
	return s
}

// splitSQLAggregate 如果一个SQL中存在集合关键词(union、union all、minus、intersect)，则需要拆分成多个单条SQL
func splitSQLAggregate(s string) ([]string, []string) {
	re := regexp.MustCompile(`( UNION ALL )|( UNION )|( MINUS )|( INTERSECT )`)
	return re.Split(s, -1), re.FindAllString(s, -1)
}

// parserSelect 对一个完整的查询SQL进行解析，返回Select的语法树
func parserSelect(s string, placeholder *[]Placeholder, placeholderPos *int) (sel Select, err error) {
	//先把集合分开，然后在处理单个查询
	sqls, keys := splitSQLAggregate(s)
	for idx, item := range sqls {
		selItem, err := parserSelectItem(item, placeholder, placeholderPos)
		if err != nil {
			return Select{}, err
		}
		if idx > 0 {
			selItem.Aggregate = keys[idx-1]
		}
		sel.Select = append(sel.Select, selItem)
	}
	return sel, nil
}

// parserSelectItem 对单查询的SQL进行解析，返回单查询的语法树
func parserSelectItem(s string, placeholder *[]Placeholder, placeholderPos *int) (sel SelectItem, err error) {
	//如果查询是被括号包裹的，先去掉外层括号
	if s[0] == '$' {
		s, _, err = getPlaceholder(s, placeholder, placeholderPos)
		if err != nil {
			return SelectItem{}, err
		}
		s = strings.TrimSpace(s)
		s = strings.TrimLeft(s, "(")
		s = strings.TrimRight(s, ")")
		return parserSelectItem(s, placeholder, placeholderPos)
	}
	s = strings.TrimSpace(s)
	s = trimLR(s, "(", ")")
	s = strings.TrimSpace(s)
	//按关键词分割语句
	selKeyword, err := splitSqlByKeywordForSelect(s)
	if err != nil {
		return SelectItem{}, err
	}
	//解析字段
	sel.Field, err = getSelectField(selKeyword.Select, placeholder, placeholderPos)
	if err != nil {
		return SelectItem{}, err
	}
	sel.Table, err = getSelectTable(selKeyword.From, placeholder, placeholderPos)
	if err != nil {
		return SelectItem{}, err
	}
	sel.Where, err = getEquationList(selKeyword.Where, placeholder, placeholderPos)
	if err != nil {
		return SelectItem{}, err
	}
	sel.Group, err = getSelectGroup(selKeyword.Group, placeholder, placeholderPos)
	if err != nil {
		return SelectItem{}, err
	}
	sel.Having, err = getEquationList(selKeyword.Having, placeholder, placeholderPos)
	if err != nil {
		return SelectItem{}, err
	}
	sel.Order, err = getSelectOrder(selKeyword.Order, placeholder, placeholderPos)
	if err != nil {
		return SelectItem{}, err
	}
	return sel, nil
}

// getTable 传入被查询的表，返回表的结构体，即：表 别名
func getTable(s string, placeholder *[]Placeholder, placeholderPos *int) (table SelectTable, err error) {
	strs := strings.Split(s, " ")
	if len(strs) == 2 {
		retStr, retPlace, err := getPlaceholder(strs[1], placeholder, placeholderPos)
		if err != nil {
			return SelectTable{}, err
		}
		table.Alias = retStr
		retStr, retPlace, err = getPlaceholder(strs[0], placeholder, placeholderPos)
		if len(retPlace) > 0 && retStr[:1] != "\"" && retStr[:1] != "`" {
			table.Table, err = parserSelect(retStr, placeholder, placeholderPos)
			if err != nil {
				return SelectTable{}, err
			}
		} else {
			table.Table = retStr
		}
	} else if len(strs) == 1 {
		retStr, retPlace, err := getPlaceholder(strs[0], placeholder, placeholderPos)
		if len(retPlace) > 0 && retStr[:1] != "\"" && retStr[:1] != "`" {
			table.Table, err = parserSelect(strs[0], placeholder, placeholderPos)
			if err != nil {
				return SelectTable{}, err
			}
		} else {
			table.Table = retStr
		}
	} else {
		return SelectTable{}, errors.New("不正确的表")
	}
	return table, nil
}

type JoinString struct {
	Table   string
	Keyword string
	On      string
}

// splitJoin 提取出join的部分，传入的字符串应确认有JOIN的存在。返回的应该是JOIN前边的表、JOIN本身、ON后面的部分
func splitJoin(s string) (rets []JoinString) {
	strs := strings.Split(s, " ")
	var tab, word, on string
	var bOn, bJoin bool
	for _, item := range strs {
		if item == "LEFT" || item == "RIGHT" || item == "INNER" {
			bJoin = true
			bOn = false
			//保存上一次的
			if tab != "" || word != "" || on != "" {
				rets = append(rets, JoinString{Table: strings.TrimSpace(tab), Keyword: strings.TrimSpace(word), On: strings.TrimSpace(on)})
			}
			word = ""
			tab = ""
			word += item + " "
		} else if item == "JOIN" {
			bOn = false
			//保存上一次的
			if !bJoin && (tab != "" || word != "" || on != "") {
				rets = append(rets, JoinString{Table: strings.TrimSpace(tab), Keyword: strings.TrimSpace(word), On: strings.TrimSpace(on)})
			}
			tab = ""
			word += "JOIN"
		} else if item == "ON" {
			bOn = true
			on = ""
		} else {
			if bOn {
				on += item + " "
			} else {
				tab += item + " "
			}
		}
	}
	if tab != "" || word != "" || on != "" {
		rets = append(rets, JoinString{Table: strings.TrimSpace(tab), Keyword: strings.TrimSpace(word), On: strings.TrimSpace(on)})
	}
	return rets
}

// getSelectOrder 解析Order排序
func getSelectOrder(s string, placeholder *[]Placeholder, placeholderPos *int) (order interface{}, err error) {
	//它可能是ORDER BY 各值；DECODE函数自定义排序
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	strType, err := getSqlType(s, placeholder, placeholderPos)
	if err != nil {
		return nil, err
	}
	if strType == "BY" {
		var orderBy OrderBy
		s = strings.Replace(s, "BY ", "", 1)
		nPos := strings.LastIndex(s, " DESC")
		if nPos != -1 {
			//说明排序是倒序
			orderBy.Collation = "DESC"
			s = strings.TrimRight(s, "DESC")
		} else {
			nPos = strings.LastIndex(s, "ASC")
			orderBy.Collation = "ASC"
			s = strings.TrimRight(s, "ASC")
		}
		orderBy.Value, err = getSelectGroup(s, placeholder, placeholderPos)
		return orderBy, err
	} else if strType == "DECODE" {
		return getValue(s, placeholder, placeholderPos)
	} else {
		return nil, errors.New("未能识别的排序规则" + strType)
	}
}

// getSelectGroup 解析分组
func getSelectGroup(s string, placeholder *[]Placeholder, placeholderPos *int) (groups []Value, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	items := strings.Split(s, ",")
	for _, item := range items {
		val, err := getValue(item, placeholder, placeholderPos)
		if err != nil {
			return nil, err
		}
		groups = append(groups, val)
	}
	return groups, nil
}

// getSelectTable 解析查询语句的表，里面包含了JOIN部分
func getSelectTable(s string, placeholder *[]Placeholder, placeholderPos *int) (tables []SelectTable, err error) {
	//查询语句的表以逗号隔开
	items := strings.Split(s, ",")
	for _, item := range items {
		var table SelectTable
		item = strings.TrimSpace(item)
		//判断表是不是存在JOIN
		joinRe := regexp.MustCompile(`( LEFT )|( RIGHT )|( INNER )|( JOIN )|( ON )`)
		joinFind := joinRe.FindAllString(item, -1)
		if len(joinFind) == 0 {
			//没有join的时候
			table, err = getTable(item, placeholder, placeholderPos)
			if err != nil {
				return nil, err
			}
		} else {
			var tabs []SelectTable
			//有join的时候
			joins := splitJoin(item)
			for _, join := range joins {
				var tab SelectTable
				tab.Table, err = getTable(join.Table, placeholder, placeholderPos)
				if err != nil {
					return nil, err
				}
				//以上取得表名
				if join.Keyword != "" {
					tab.JoinKey = join.Keyword
					tab.JoinOn, err = getEquationList(join.On, placeholder, placeholderPos)
				}
				tabs = append(tabs, tab)
			}
			table.Table = tabs
		}
		tables = append(tables, table)
	}
	return tables, nil
}

// getSelectField 解析查询语句的字段部分
func getSelectField(s string, placeholder *[]Placeholder, placeholderPos *int) (fields []SelectField, err error) {
	items := strings.Split(s, ",")
	for _, item := range items {
		//按空格拆分
		item = strings.TrimSpace(item)
		fs := strings.Split(item, " ")
		var field SelectField
		switch len(fs) {
		case 1:
			//一定是字段本身
			field.Field, err = getValue(fs[0], placeholder, placeholderPos)
			if err != nil {
				return nil, err
			}
		default:
			//看看最后一个是不是别名
			var alias string
			alias, _, err = getPlaceholder(fs[len(fs)-1], placeholder, placeholderPos)
			if alias[0] == '\'' || alias[0] == '`' || fs[len(fs)-1][0] != '$' {
				//是别名
				field.Alias = alias
				pos := strings.LastIndex(item, fs[len(fs)-1])
				field.Field, err = getValue(item[:pos], placeholder, placeholderPos)
				if err != nil {
					return nil, err
				}
			} else {
				field.Field, err = getValue(item, placeholder, placeholderPos)
			}
		}
		fields = append(fields, field)
	}
	return fields, nil
}

// getValue 解析成Value SQL的值，它可以是子查询、函数、CASE WHEN表达式、字符串、数字（应当包括加减乘除等运算）、字段TableField（即不被括号括起来的，包含了像SYSDATE这样的关键词）、参数、被双竖线连接的值组合；它可以出现在：查询的字段、条件语句的左右值、新增/更新语句的值
func getValue(s string, placeholder *[]Placeholder, placeholderPos *int) (value Value, err error) {
	s = strings.TrimSpace(s)
	//如果不是子查询，才会生效这个连接符，因为当整个值是一个子查询的话，那么里面的双竖线就是子查询里面的
	if strings.Index(s, "SELECT ") != 0 {
		//首先需要用||分割开
		items := strings.Split(s, "||")
		if len(items) > 1 {
			//说明它是用竖线连接的字符串
			var retVal []Value
			for _, item := range items {
				item = strings.TrimSpace(item)
				val, err := getValue(item, placeholder, placeholderPos)
				if err != nil {
					return Value{}, err
				}
				retVal = append(retVal, val)
			}
			value.Value = retVal
			return value, nil
		}
	}
	//只有一项，则看是什么东西
	numItems := getNumberItemKeyword(s)
	if len(numItems) > 1 {
		//说明是有加减乘除运算的
		var retVal Number
		for _, item := range numItems {
			item.Value = strings.TrimSpace(item.Value)
			val, err := getValue(item.Value, placeholder, placeholderPos)
			if err != nil {
				return Value{}, err
			}
			retVal.Number = append(retVal.Number, NumberItem{Value: val, Operator: item.Operator})
		}
		value.Value = retVal
		return value, nil
	}
	//如果该项是单值，则可能是子查询、函数、CASE WHEN表达式、参数、普通字符串
	strs := strings.Split(s, " ")
	switch len(strs) {
	case 1:
		//只有一项的时候，它可能是普通字符串、子查询、参数
		retStr, retPlace, err := getPlaceholder(strs[0], placeholder, placeholderPos)
		if err != nil {
			return Value{}, err
		}
		if len(retPlace) == 0 {
			//说明是普通字符串
			value.Value = retStr
		} else if len(retPlace) == 1 {
			if retStr[0] == ':' {
				//说明是参数
				value.Value = Params{Name: retStr}
			} else if retStr[0] == '\'' || retStr[0] == '"' || retStr[0] == '`' {
				value.Value = retStr
			} else {
				//说明是子查询，或者是被括号括起的表达式
				strs[0] = trimLR(retPlace[0].Value, "(", ")")
				strType, err := getSqlType(strs[0], placeholder, placeholderPos)
				if err != nil {
					return Value{}, err
				}
				if strType == "SELECT" {
					value.Value, err = parserSelect(strs[0], placeholder, placeholderPos)
					if err != nil {
						return Value{}, err
					}
				} else {
					value.Value, err = getValue(strs[0], placeholder, placeholderPos)
					if err != nil {
						return Value{}, err
					}
				}
			}
		} else {
			return Value{}, errors.New("SQL值可能有误")
		}
	case 2:
		//两项的时候，那他一定是函数
		value.Value, err = getFunction(strs[0], strs[1], placeholder, placeholderPos)
		if err != nil {
			return Value{}, err
		}
	default:
		//其他情况应该看看是不是case when
		if strs[0] == "CASE" {
			value.Value, err = getCaseWhen(s, placeholder, placeholderPos)
			if err != nil {
				return Value{}, err
			}
		} else if strs[0] == "SELECT" {
			value.Value, err = parserSelect(s, placeholder, placeholderPos)
			if err != nil {
				return Value{}, err
			}
		} else {
			return Value{}, errors.New("SQL值存在不能解析的元素")
		}
	}
	return value, nil
}

// getFunction 解析函数的部分
func getFunction(name, params string, placeholder *[]Placeholder, placeholderPos *int) (f Function, err error) {
	nameStr, _, err := getPlaceholder(name, placeholder, placeholderPos)
	if err != nil {
		return Function{}, err
	}
	f.Name = nameStr
	paramsStr, _, err := getPlaceholder(params, placeholder, placeholderPos)
	//去掉首尾括号
	paramsStr = strings.TrimSpace(paramsStr)
	paramsStr = strings.TrimLeft(paramsStr, "(")
	paramsStr = strings.TrimRight(paramsStr, ")")
	paramsStr = strings.TrimSpace(paramsStr)
	//按逗号取出里面的参数值
	strs := strings.Split(paramsStr, ",")
	for _, item := range strs {
		val, err := getValue(item, placeholder, placeholderPos)
		if err != nil {
			return Function{}, err
		}
		f.Params = append(f.Params, val)
	}
	return f, nil
}

// replaceBetweenItem 替换between and表达式为$BETWEEN
func replaceBetweenItem(s string) (newStr string, betStr string, err error) {
	newStr = ""
	betStr = ""
	err = errors.New("未知错误")
	nBet := strings.Index(s, " BETWEEN ")
	if nBet == -1 {
		return s, betStr, nil
	}
	//存在between
	nStart := strings.LastIndex(s[:nBet], " AND ")
	if nStart == -1 {
		nStart = strings.LastIndex(s[:nBet], " OR ")
		if nStart != -1 {
			nStart += 4
		}
	} else {
		nStart += 5
	}
	if nStart == -1 {
		nStart = 0
	}
	nAnd := strings.Index(s[nBet:], " AND ")
	if nAnd == -1 {
		err = errors.New("BETWEEN表达式缺失AND关键词")
		return
	} else {
		nAnd += nBet
	}
	nEnd := strings.Index(s[nAnd+4:], " AND ")
	if nEnd == -1 {
		nEnd = strings.Index(s[nAnd+4:], " OR ")
	} else {
		nEnd += nAnd + 4
	}
	if nEnd == -1 {
		nEnd = len(s)
	}
	betStr = s[nStart:nEnd]
	newStr = s[:nStart] + "$BETWEEN" + s[nEnd:]
	err = nil
	return
}

func replaceBetween(s string) (newStr string, betStrs []string, err error) {
	newStr = s
	for {
		ns, bs, err := replaceBetweenItem(newStr)
		if err != nil {
			return "", nil, err
		}
		newStr = ns
		if bs == "" {
			break
		}
		betStrs = append(betStrs, bs)
	}
	return newStr, betStrs, nil
}

// getEquationList 解析条件部分
func getEquationList(s string, placeholder *[]Placeholder, placeholderPos *int) (list EquationList, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return EquationList{}, nil
	}
	//BETWEEN AND先用占位符替换
	//re := regexp.MustCompile(`\$?\w*((\|\|\$?\w*)+)? BETWEEN \$?\w*((\|\|\$?\w*)+)? AND \$?\w*((\|\|\$?\w*)+)?`)
	//ph := re.FindAllString(s, -1)
	//var phNum int
	//s = re.ReplaceAllString(s, "$BETWEEN")
	var phNum int
	var ph []string
	s, ph, err = replaceBetween(s)
	if err != nil {
		return EquationList{}, err
	}
	//用AND OR分割
	resp := regexp.MustCompile(`( AND )|( OR )`)
	matches := resp.FindAllString(s, -1)
	strs := resp.Split(s, -1)
	for idx, item := range strs {
		item = strings.TrimSpace(item)
		var equation Equation
		if idx > 0 {
			equation.Connector = strings.TrimSpace(matches[idx-1])
		}
		//item就是一个条件/一组条件。它可能有$BETWEEN，和括号括起来的条件组
		if item == "$BETWEEN" {
			//BETWEEN AND表达式
			reBetween := regexp.MustCompile(`(BETWEEN)|(AND)`)
			betVals := reBetween.Split(ph[phNum], -1)
			phNum++
			var eqBet EquationBetween
			eqBet.Field, err = getValue(betVals[0], placeholder, placeholderPos)
			if err != nil {
				return EquationList{}, err
			}
			eqBet.Left, err = getValue(betVals[1], placeholder, placeholderPos)
			if err != nil {
				return EquationList{}, err
			}
			eqBet.Right, err = getValue(betVals[2], placeholder, placeholderPos)
			if err != nil {
				return EquationList{}, err
			}
			equation.Equation = eqBet
			list.Equation = append(list.Equation, equation)
			continue
		}
		newSql, placeList, err := getPlaceholder(item, placeholder, placeholderPos)
		if err != nil {
			return EquationList{}, err
		}
		if len(placeList) == 1 && placeList[0].Name == item {
			equation.Equation, err = getEquationList(newSql, placeholder, placeholderPos)
			if err != nil {
				return EquationList{}, err
			}
		} else {
			//正常的条件，有可能是各种符号，和IN、NOT IN、EXIST、NOT EXIST、LIKE、NOT LIKE等
			//先处理常规比较符的
			reNorm := regexp.MustCompile(`>|=|<|!`)
			normOperators := reNorm.FindAllString(item, -1)
			normStrs := reNorm.Split(item, -1)
			if len(normOperators) > 0 {
				if len(normStrs) != 2 {
					return EquationList{}, errors.New("常规比较式需要有左值和右值")
				}
				//说明是常规的比较式
				var eqNorm EquationNorm
				eqNorm.Left, err = getValue(normStrs[0], placeholder, placeholderPos)
				if err != nil {
					return EquationList{}, err
				}
				eqNorm.Right, err = getValue(normStrs[1], placeholder, placeholderPos)
				if err != nil {
					return EquationList{}, err
				}
				for _, o := range normOperators {
					eqNorm.Operator += o
				}
				equation.Equation = eqNorm
			} else {
				//可能是IN、NOT IN、EXIST、NOT EXIST、LIKE、NOT LIKE等
				var eqOther EquationOther
				reOther := regexp.MustCompile(`\bIN\b|\bNOT IN\b|\bLIKE\b|\bNOT LIKE\b|\bEXIST\b|\bNOT EXIST\b|\bIS NULL\b|\bIS NOT NULL\b`)
				otherKeys := reOther.FindAllString(item, -1)
				otherVals := reOther.Split(item, -1)
				for _, o := range otherKeys {
					eqOther.Operator += o + " "
				}
				eqOther.Operator = strings.TrimSpace(eqOther.Operator)
				eqOther.Left, err = getValue(otherVals[0], placeholder, placeholderPos)
				if err != nil {
					return EquationList{}, err
				}
				//如果是IN/EXIST带括号的，可能有多个值，如果是LIKE，就一个值，NULL没有值
				if strings.Index(eqOther.Operator, "IN") > -1 || strings.Index(eqOther.Operator, "EXIST") > -1 {
					otherVals[1], _, _ = getPlaceholder(otherVals[1], placeholder, placeholderPos)
					otherVals[1] = strings.TrimSpace(otherVals[1])
					otherVals[1] = trimLR(otherVals[1], "(", ")")
					wheres := strings.Split(otherVals[1], ",")
					for _, whe := range wheres {
						var itemVal Value
						itemVal, err = getValue(whe, placeholder, placeholderPos)
						if err != nil {
							return EquationList{}, err
						}
						eqOther.Right = append(eqOther.Right, itemVal)
					}
				} else if strings.Index(eqOther.Operator, "LIKE") > -1 {
					var itemVal Value
					itemVal, err = getValue(otherVals[1], placeholder, placeholderPos)
					eqOther.Right = append(eqOther.Right, itemVal)
				}
				equation.Equation = eqOther
			}
		}
		list.Equation = append(list.Equation, equation)
	}
	return list, nil
}

// getCaseWhen 解析Case when表达式
func getCaseWhen(s string, placeholder *[]Placeholder, placeholderPos *int) (cas CaseWhen, err error) {
	s = strings.TrimSpace(s)
	re := regexp.MustCompile(`\bCASE\b|\bWHEN\b|\bTHEN\b|\bELSE\b|\bEND\b`)
	matches := re.FindAllString(s, -1)
	strs := re.Split(s, -1)
	//去掉strs的空串
	var newStrs []string
	for _, item := range strs {
		if strings.TrimSpace(item) != "" {
			newStrs = append(newStrs, strings.TrimSpace(item))
		}
	}
	strs = newStrs
	bCaseWhen := false
	if strings.Index(s, "CASE WHEN") > -1 {
		bCaseWhen = true
	}
	var caseVal Value
	var caseItem CaseWhenItem
	var equationNorm EquationNorm
	var equationList EquationList
	bEnd := false
	var idx int
	for _, key := range matches {
		switch key {
		case "CASE":
			if !bCaseWhen {
				caseVal, err = getValue(strs[idx], placeholder, placeholderPos)
				idx++
				if err != nil {
					return CaseWhen{}, err
				}
			}
		case "WHEN":
			equationNorm = EquationNorm{}
			equationList = EquationList{}
			caseItem = CaseWhenItem{}
			if !bCaseWhen {
				equationNorm.Left = caseVal
				equationNorm.Operator = "="
				equationNorm.Right, err = getValue(strs[idx], placeholder, placeholderPos)
				idx++
				if err != nil {
					return CaseWhen{}, err
				}
			} else {
				equationList, err = getEquationList(strs[idx], placeholder, placeholderPos)
				idx++
				if err != nil {
					return CaseWhen{}, err
				}
			}
		case "THEN":
			caseItem.Value, err = getValue(strs[idx], placeholder, placeholderPos)
			idx++
			if err != nil {
				return CaseWhen{}, err
			}
			if !bCaseWhen {
				caseItem.Equation.Equation = append(caseItem.Equation.Equation, Equation{Equation: equationNorm})
			} else {
				caseItem.Equation = equationList
			}
			cas.When = append(cas.When, caseItem)
		case "ELSE":
			cas.Else, err = getValue(strs[idx], placeholder, placeholderPos)
			idx++
			if err != nil {
				return CaseWhen{}, err
			}
		case "END":
			bEnd = true
		default:
			return CaseWhen{}, errors.New("CASE WHEN表达式出现未知关键词")
		}
	}
	if !bEnd {
		return CaseWhen{}, errors.New("CASE WHEN表达式缺少END")
	}
	return cas, nil
}

type NumberItemKeyword struct {
	Value    string
	Operator string
}

// getNumberItemKeyword 将字符串按照+-*/分割
func getNumberItemKeyword(s string) (items []NumberItemKeyword) {
	re := regexp.MustCompile(`\+|-|\*|/`)
	matches := re.FindAllString(s, -1)
	strs := re.Split(s, -1)
	if len(matches) == 1 && matches[0] == "*" {
		if strings.Index(s, ".*") > -1 {
			return nil
		}
		if len(strs) != 2 {
			return nil
		}
		if strs[0] == "" || strs[1] == "" {
			return nil
		}
	}
	for idx, item := range strs {
		var numItem NumberItemKeyword
		numItem.Value = item
		if idx > 0 {
			numItem.Operator = matches[idx-1]
		}
		items = append(items, numItem)
	}
	return items
}

type SelectKeyword struct {
	Select string
	From   string
	Where  string
	Group  string
	Having string
	Order  string
}

type KeyPos struct {
	Key   string
	Start int
	End   int
}

// FindKeyPos 查询所有key的位置
func FindKeyPos(s string, keys []KeyPos) []KeyPos {
	for i := 0; i < len(keys); i++ {
		keys[i].Start = strings.Index(s, keys[i].Key)
		if keys[i].Start != -1 {
			keys[i].End = keys[i].Start + len(keys[i].Key)
		}
	}
	return keys
}

// splitSqlByKeywordForSelect 按查询的关键词(SELECT FROM WHERE GROUP ORDER)分割SQL
func splitSqlByKeywordForSelect(s string) (sel SelectKeyword, err error) {
	var selectKey, fromKey, whereKey, groupKey, orderKey, havingKey KeyPos
	selectKey.Key = "SELECT "
	fromKey.Key = " FROM "
	whereKey.Key = " WHERE "
	groupKey.Key = " GROUP BY "
	havingKey.Key = " HAVING "
	orderKey.Key = " ORDER "
	keys := []KeyPos{selectKey, fromKey, whereKey, groupKey, havingKey, orderKey}
	keys = FindKeyPos(s, keys)
	sort.SliceStable(keys, func(i, j int) bool {
		return keys[i].Start < keys[j].Start
	})
	//排序后，按非-1开始的关键词开始
	for i := 0; i < len(keys); i++ {
		if keys[i].Start == -1 {
			continue
		}
		//查询的关键词部分，一定是从SELECT ... FROM ... 从这以后就是不一定有的，如果有，顺序一定是WHERE ... GROUP/HAVING ... ORDER ...
		if keys[i].Key == "SELECT " {
			if i == len(keys)-1 {
				return sel, errors.New("未找到FROM关键词")
			}
			sel.Select = s[keys[i].End:keys[i+1].Start]
		} else if keys[i].Key == " FROM " {
			if i == len(keys)-1 {
				sel.From = s[keys[i].End:]
			} else {
				sel.From = s[keys[i].End:keys[i+1].Start]
			}
		} else if keys[i].Key == " WHERE " {
			if i == len(keys)-1 {
				sel.Where = s[keys[i].End:]
			} else {
				sel.Where = s[keys[i].End:keys[i+1].Start]
			}
		} else if keys[i].Key == " GROUP BY " {
			if i == len(keys)-1 {
				sel.Group = s[keys[i].End:]
			} else {
				sel.Group = s[keys[i].End:keys[i+1].Start]
			}
		} else if keys[i].Key == " HAVING " {
			if i == len(keys)-1 {
				sel.Having = s[keys[i].End:]
			} else {
				sel.Having = s[keys[i].End:keys[i+1].Start]
			}
		} else if keys[i].Key == " ORDER " {
			if i == len(keys)-1 {
				sel.Order = s[keys[i].End:]
			} else {
				sel.Order = s[keys[i].End:keys[i+1].Start]
			}
		}
	}
	if sel.Select == "" {
		return sel, errors.New("字段缺失")
	}
	if sel.From == "" {
		return sel, errors.New("表缺失")
	}
	return sel, nil
}

// parserInsert 解析插入语句
func parserInsert(s string, placeholder *[]Placeholder, placeholderPos *int) (insert Insert, err error) {
	intoPos := strings.Index(s, "INSERT INTO ")
	if intoPos == -1 {
		return Insert{}, errors.New("缺失INTO关键词")
	}
	valuesPos := strings.Index(s, "VALUES ")
	selectPos := -1
	if valuesPos == -1 {
		//看看是否存在select
		selectPos = strings.Index(s, "SELECT ")
		if selectPos == -1 {
			return Insert{}, errors.New("缺失VALUES关键词")
		}
	}
	tabEnd := -1
	if valuesPos != -1 {
		tabEnd = valuesPos
	} else {
		tabEnd = selectPos
	}
	tabStrs := strings.Split(strings.TrimSpace(s[intoPos+len("INSERT INTO "):tabEnd]), " ")
	if len(tabStrs) == 0 || tabStrs[0] == "" {
		return Insert{}, errors.New("缺失表名")
	}
	retStr, _, err := getPlaceholder(tabStrs[0], placeholder, placeholderPos)
	if err != nil {
		return Insert{}, err
	}
	insert.Table = retStr
	if len(tabStrs) == 2 {
		//说明含有字段
		fieldStr, _, err := getPlaceholder(tabStrs[1], placeholder, placeholderPos)
		if err != nil {
			return Insert{}, err
		}
		fieldStr = trimLR(strings.TrimSpace(fieldStr), "(", ")")
		fields := strings.Split(fieldStr, ",")
		for _, item := range fields {
			insert.Field = append(insert.Field, strings.TrimSpace(item))
		}
	} else if len(tabStrs) > 2 {
		return Insert{}, errors.New("被插入的表和字段不正确")
	}
	vals := ""
	if valuesPos != -1 {
		//values的形式
		vals, _, err = getPlaceholder(strings.TrimSpace(s[valuesPos+len("VALUES "):]), placeholder, placeholderPos)
		vals = trimLR(vals, "(", ")")
		valItems := strings.Split(vals, ",")
		var values []Value
		for _, item := range valItems {
			val, err := getValue(strings.TrimSpace(item), placeholder, placeholderPos)
			if err != nil {
				return Insert{}, err
			}
			values = append(values, val)
		}
		insert.Values = values
	}
	if selectPos != -1 {
		//select的形式
		insert.Values, err = parserSelect(s[selectPos:], placeholder, placeholderPos)
	}
	return insert, nil
}

// parserUpdate 解析更新语句
func parserUpdate(s string, placeholder *[]Placeholder, placeholderPos *int) (update Update, err error) {
	setPos := strings.Index(s, " SET ")
	if setPos == -1 {
		return Update{}, errors.New("缺失SET关键词")
	}
	//where不一定有
	wherePos := strings.Index(s, " WHERE ")
	update.Table, _, err = getPlaceholder(strings.TrimSpace(s[len("UPDATE"):setPos]), placeholder, placeholderPos)
	nSetEnd := wherePos
	if wherePos == -1 {
		nSetEnd = len(s)
	}
	setItems := strings.Split(s[setPos+len(" SET"):nSetEnd], ",")
	for _, item := range setItems {
		eqStrs := strings.Split(item, "=")
		if len(eqStrs) != 2 {
			return Update{}, errors.New("UPDATE设置值必须是等式")
		}
		var setItem UpdateValueItem
		setItem.Field, _, err = getPlaceholder(strings.TrimSpace(eqStrs[0]), placeholder, placeholderPos)
		if err != nil {
			return Update{}, err
		}
		setItem.Value, err = getValue(strings.TrimSpace(eqStrs[1]), placeholder, placeholderPos)
		if err != nil {
			return Update{}, err
		}
		update.Value = append(update.Value, setItem)
	}
	if wherePos != -1 {
		update.Where, err = getEquationList(strings.TrimSpace(s[wherePos+len(" WHERE"):]), placeholder, placeholderPos)
		if err != nil {
			return Update{}, err
		}
	}
	return update, nil
}

func parserDelete(s string, placeholder *[]Placeholder, placeholderPos *int) (delete Delete, err error) {
	fromPos := strings.Index(s, " FROM ")
	wherePos := strings.Index(s, " WHERE ")
	nTabStart := fromPos
	if nTabStart == -1 {
		nTabStart = len("DELETE ")
	} else {
		nTabStart += len(" FROM")
	}
	nTabEnd := wherePos
	if nTabEnd == -1 {
		nTabEnd = len(s)
	}
	delete.Table, _, err = getPlaceholder(strings.TrimSpace(s[nTabStart:nTabEnd]), placeholder, placeholderPos)
	if err != nil {
		return Delete{}, err
	}
	if wherePos != -1 {
		delete.Where, err = getEquationList(strings.TrimSpace(s[wherePos+len(" WHERE"):]), placeholder, placeholderPos)
		if err != nil {
			return Delete{}, err
		}
	}
	return delete, nil
}

// marshalFunction 序列化函数
func marshalFunction(function Function) (retSQL string, err error) {
	retSQL += function.Name + "("
	for _, item := range function.Params {
		val, err := marshalValue(item, true)
		if err != nil {
			return "", err
		}
		retSQL += val + ","
	}
	retSQL = strings.TrimRight(retSQL, ",") + ")"
	return retSQL, nil
}

// marshalEquationNorm 序列化常态的条件
func marshalEquationNorm(eq EquationNorm) (retSQL string, err error) {
	if eq.Operator != "<" && eq.Operator != "<=" && eq.Operator != ">" && eq.Operator != ">=" && eq.Operator != "=" && eq.Operator != "!=" && eq.Operator != "<>" {
		return "", errors.New("比较式的符合" + eq.Operator + "不符合规则")
	}
	lv, err := marshalValue(eq.Left, true)
	if err != nil {
		return "", err
	}
	rv, err := marshalValue(eq.Right, true)
	if err != nil {
		return "", err
	}
	retSQL = lv + eq.Operator + rv
	return retSQL, nil
}

// marshalEquationOther 序列化其他条件
func marshalEquationOther(eq EquationOther) (retSQL string, err error) {
	if eq.Operator != "IS NULL" && eq.Operator != "IS NOT NULL" && eq.Operator != "IN" && eq.Operator != "NOT IN" && eq.Operator != "EXIST" && eq.Operator != "NOT EXIST" && eq.Operator != "LIKE" && eq.Operator != "NOT LIKE" {
		return "", errors.New("条件" + eq.Operator + "无效")
	}
	lv, err := marshalValue(eq.Left, true)
	if err != nil {
		return "", err
	}
	lrStr := "("
	for _, item := range eq.Right {
		val, err := marshalValue(item, true)
		if err != nil {
			return "", err
		}
		lrStr += val + ","
	}
	lrStr = strings.TrimRight(lrStr, ",")
	lrStr += ")"
	if lrStr != "()" {
		if eq.Operator == "LIKE" || eq.Operator == "NOT LIKE" {
			lrStr = trimLR(lrStr, "(", ")")
			retSQL = lv + " " + eq.Operator + " " + lrStr
		} else {
			retSQL = lv + " " + eq.Operator + lrStr
		}
	} else {
		retSQL = lv + " " + eq.Operator
	}
	return retSQL, nil
}

func marshalEquationBetween(eq EquationBetween) (retSQL string, err error) {
	fld, err := marshalValue(eq.Field, true)
	if err != nil {
		return "", err
	}
	lv, err := marshalValue(eq.Left, true)
	if err != nil {
		return "", err
	}
	rv, err := marshalValue(eq.Right, true)
	if err != nil {
		return "", err
	}
	return fld + " BETWEEN " + lv + " AND " + rv, nil
}

func marshalEquationList(eqList EquationList) (retSQL string, err error) {
	if len(eqList.Equation) == 0 {
		return "", nil
	}
	for _, item := range eqList.Equation {
		//外层的不带括号，如果遇上了EquationList，需带上括号
		eqStr := ""
		switch v := item.Equation.(type) {
		case EquationNorm:
			eqStr, err = marshalEquationNorm(v)
		case EquationOther:
			eqStr, err = marshalEquationOther(v)
		case EquationBetween:
			eqStr, err = marshalEquationBetween(v)
		case EquationList:
			eqStr, err = marshalEquationList(v)
			eqStr = "(" + eqStr + ")"
		default:
			return "", errors.New("条件列表存在未能识别的类型")
		}
		if err != nil {
			return "", err
		}
		retSQL += " " + item.Connector + " " + eqStr
	}
	return strings.TrimSpace(retSQL), nil
}

// marshalCaseWhenItem 序列化case when表达式的when项
func marshalCaseWhenItem(whenItem CaseWhenItem) (retSQL string, err error) {
	retSQL += "WHEN "
	eqList, err := marshalEquationList(whenItem.Equation)
	if err != nil {
		return "", err
	}
	retSQL += eqList + " THEN "
	val, err := marshalValue(whenItem.Value, true)
	if err != nil {
		return "", err
	}
	retSQL += val
	return retSQL, nil
}

// marshalCaseWhen 序列化case when表达式
func marshalCaseWhen(caseWhen CaseWhen) (retSQL string, err error) {
	if len(caseWhen.When) == 0 {
		return "", errors.New("CASE WHEN表达式需要有WHEN项")
	}
	retSQL = "CASE "
	for _, item := range caseWhen.When {
		itemStr, err := marshalCaseWhenItem(item)
		if err != nil {
			return "", err
		}
		retSQL += itemStr + " "
	}
	if caseWhen.Else.Value != nil {
		elseVal, err := marshalValue(caseWhen.Else, true)
		if err != nil {
			return "", err
		}
		retSQL += "ELSE " + elseVal + " "
	}
	retSQL += "END"
	return retSQL, nil
}

// marshalNumber 序列化数字
func marshalNumber(num Number) (retSQL string, err error) {
	if len(num.Number) == 0 {
		return "", errors.New("数字值不能为空")
	}
	for _, item := range num.Number {
		retNum, err := marshalValue(item.Value, true)
		if err != nil {
			return "", err
		}
		retSQL += item.Operator + retNum
	}
	if len(num.Number) > 1 {
		retSQL = "(" + retSQL + ")"
	}
	return retSQL, err
}

// marshalParams 序列换参数
func marshalParams(par Params) (retSQL string, err error) {
	if par.Name == "" {
		return "", errors.New("参数名称不能为空")
	}
	if par.Name[0] != ':' {
		return "", errors.New("参数名必须是:开头")
	}
	return par.Name, nil
}

// marshalValue 序列化值，top顶层值，非双竖线连接的字符串，都应该是顶层值，true
func marshalValue(value Value, top bool) (retSQL string, err error) {
	switch v := value.Value.(type) {
	case Select:
		retSQL, err = marshalSelect(v)
		if top {
			retSQL = "(" + retSQL + ")" //在in、function等里面，涉及到多个值的情况下，SQL需要用括号括起，只有最顶层的值才需要被括号括起
		}
		return retSQL, err
	case Function:
		return marshalFunction(v)
	case CaseWhen:
		return marshalCaseWhen(v)
	case string:
		return v, nil
	case Number:
		return marshalNumber(v)
	case Params:
		return marshalParams(v)
	case Value:
		return marshalValue(v, true)
	case nil:
		return "NULL", nil
		//return "", errors.New("值不能为空")
	case []Value:
		for _, item := range v {
			val, err := marshalValue(item, true)
			if err != nil {
				return "", err
			}
			//if strings.Index(val, "SELECT ") == 0{
			//	val = "(" + val + ")"
			//}
			retSQL += val + "||"
		}
		retSQL = strings.TrimRight(retSQL, "||")
	default:
		return "", errors.New("值存在不能识别的类型")
	}
	return retSQL, err
}

// marshalSelectFieldList 序列化查询的字段
func marshalSelectFieldList(fields []SelectField) (retSQL string, err error) {
	if len(fields) == 0 {
		return "", errors.New("缺失字段")
	}
	for _, item := range fields {
		fldStr, err := marshalValue(item.Field, true)
		if err != nil {
			return "", err
		}
		if item.Alias != "" {
			retSQL += fldStr + " " + item.Alias + ","
		} else {
			retSQL += fldStr + ","
		}
	}
	//去除最后一个逗号
	return strings.TrimRight(retSQL, ","), nil
}

// marshalSelectTable 解析表
func marshalSelectTable(tables interface{}) (retSQL string, err error) {
	//表可能是字符串，也可能是子查询，子查询需要用括号括起
	switch v := tables.(type) {
	case string:
		retSQL = v
	case Select:
		retSQL, err = marshalSelect(v)
		retSQL = "(" + retSQL + ")"
	case []SelectTable:
		retSQL, err = marshalSelectTableList(v)
	case SelectTable:
		retSQL, err = marshalSelectTable(v.Table)
	case nil:
		return "", errors.New("表不能为空")
	default:
		return "", errors.New("存在未知类型的表")
	}
	return retSQL, nil
}

// marshalSelectTableList 序列化表列表
func marshalSelectTableList(tables []SelectTable) (retSQL string, err error) {
	if len(tables) == 0 {
		return "", errors.New("缺失要查询的表")
	}
	//每张表用逗号隔开
	for _, item := range tables {
		if item.JoinKey == "JOIN" || item.JoinKey == "INNER JOIN" || item.JoinKey == "LEFT JOIN" || item.JoinKey == "RIGHT JOIN" {
			//存在正常的JOIN关系
			retSQL = strings.TrimRight(retSQL, ",")
			retSQL += " " + item.JoinKey + " "
		} else if item.JoinKey != "" {
			return "", errors.New("存在不能识别的连表查询" + item.JoinKey)
		}
		tabStr, err := marshalSelectTable(item.Table)
		if err != nil {
			return "", err
		}
		if item.Alias != "" || item.JoinKey != "" {
			retSQL += tabStr + " "
		} else {
			retSQL += tabStr
		}
		retSQL += item.Alias
		if item.JoinKey != "" {
			eqList, err := marshalEquationList(item.JoinOn)
			if err != nil {
				return "", err
			}
			retSQL += "ON " + eqList
		}
		retSQL += ","
	}
	retSQL = strings.TrimRight(retSQL, ",")
	return retSQL, nil
}

// marshalSelectItem 序列化单查询SQL
func marshalSelectItem(sel SelectItem) (retSQL string, err error) {
	retSQL += "SELECT "
	fieldStr, err := marshalSelectFieldList(sel.Field)
	if err != nil {
		return "", err
	}
	retSQL += fieldStr + " FROM "
	tableStr, err := marshalSelectTableList(sel.Table)
	if err != nil {
		return "", err
	}
	retSQL += tableStr + " "
	//看有没有where
	if sel.Where.Equation != nil {
		whereStr, err := marshalEquationList(sel.Where)
		if err != nil {
			return "", err
		}
		retSQL += "WHERE " + whereStr + " "
	}
	//看有没有group
	if len(sel.Group) > 0 {
		groupStr := ""
		for _, item := range sel.Group {
			val, err := marshalValue(item, true)
			if err != nil {
				return "", err
			}
			groupStr += val + ","
		}
		groupStr = strings.TrimRight(groupStr, ",")
		retSQL += "GROUP BY " + groupStr + " "
	}
	//看有没有having
	if len(sel.Having.Equation) > 0 {
		havingStr, err := marshalEquationList(sel.Having)
		if err != nil {
			return "", err
		}
		retSQL += "HAVING " + havingStr + " "
	}
	//看有没有order
	if sel.Order != nil {
		orderStr := ""
		switch v := sel.Order.(type) {
		case OrderBy:
			if len(v.Value) == 0 {
				return "", errors.New("order by字段不能为空")
			}
			for _, item := range v.Value {
				val, err := marshalValue(item, true)
				if err != nil {
					return "", err
				}
				orderStr += val + ","
			}
			orderStr = strings.TrimRight(orderStr, ",")
			orderStr = "ORDER BY " + orderStr + " " + v.Collation
		case Function:
			orderStr, err = marshalFunction(v)
			if err != nil {
				return "", err
			}
			orderStr = "ORDER " + orderStr
		default:
			return "", errors.New("排序类型不正确")
		}
		retSQL += orderStr
	}
	return retSQL, nil
}

// marshalSelect 序列化查询SQL
func marshalSelect(sel Select) (retSQL string, err error) {
	for _, item := range sel.Select {
		retSQL += item.Aggregate + " "
		itemSQL := ""
		itemSQL, err = marshalSelectItem(item)
		if err != nil {
			return "", err
		}
		retSQL += itemSQL + " "
	}
	return strings.TrimSpace(removeExtraSpaces(retSQL)), nil
}

// marshalInsert 序列化新增SQL
func marshalInsert(insert Insert) (retSQL string, err error) {
	if insert.Table == "" {
		return "", errors.New("INSERT语句表缺失")
	}
	if len(insert.Field) == 0 {
		retSQL += "INSERT INTO " + insert.Table + " "
	} else {
		retSQL += "INSERT INTO " + insert.Table + "("
		for _, item := range insert.Field {
			retSQL += item + ","
		}
		retSQL = strings.TrimRight(retSQL, ",")
		retSQL += ") "
	}
	switch v := insert.Values.(type) {
	case []Value:
		valStr := ""
		for _, item := range v {
			val, err := marshalValue(item, true)
			if err != nil {
				return "", err
			}
			valStr += val + ","
		}
		valStr = strings.TrimRight(valStr, ",")
		retSQL += "VALUES(" + valStr + ")"
	case Select:
		selStr := ""
		selStr, err = marshalSelect(v)
		if err != nil {
			return "", err
		}
		retSQL += selStr
	case nil:
		return "", errors.New("缺失Value值")
	default:
		return "", errors.New("不受支持的Value值")
	}
	return retSQL, nil
}

// marshalUpdate 序列化更新语句
func marshalUpdate(update Update) (retSQL string, err error) {
	if len(update.Value) == 0 {
		return "", errors.New("UPDATE语句缺失SET字段")
	}
	if update.Table == "" {
		return "", errors.New("UPDATE语句表缺失")
	}
	retSQL += "UPDATE " + update.Table + " SET "
	for _, item := range update.Value {
		val, err := marshalValue(item.Value, true)
		if err != nil {
			return "", err
		}
		if item.Field == "" {
			return "", errors.New("被SET的字段不能为空")
		}
		retSQL += item.Field + "=" + val + ","
	}
	retSQL = strings.TrimRight(retSQL, ",")
	if len(update.Where.Equation) != 0 {
		whereStr, err := marshalEquationList(update.Where)
		if err != nil {
			return "", err
		}
		retSQL += " WHERE " + whereStr
	}
	return retSQL, nil
}

// marshalDelete 序列化删除语句
func marshalDelete(delete Delete) (retSQL string, err error) {
	if delete.Table == "" {
		return "", errors.New("DELETE语句表缺失")
	}
	retSQL += "DELETE FROM " + delete.Table
	if len(delete.Where.Equation) != 0 {
		whereStr, err := marshalEquationList(delete.Where)
		if err != nil {
			return "", err
		}
		retSQL += " WHERE " + whereStr
	}
	return retSQL, nil
}

// Unmarshal 将SQL解析成语法树
func Unmarshal(s string) (stmt Statement, err error) {
	var placeholder []Placeholder
	var placeholderPos int
	s, err = placeholderByString(s, &placeholder, &placeholderPos)
	if err != nil {
		return Statement{}, err
	}
	//先判断SQL的类别
	strType, err := getSqlType(s, &placeholder, &placeholderPos)
	if err != nil {
		return Statement{}, err
	}
	switch strType {
	case "SELECT":
		stmt.Ast, err = parserSelect(s, &placeholder, &placeholderPos)
	case "UPDATE":
		stmt.Ast, err = parserUpdate(s, &placeholder, &placeholderPos)
	case "INSERT":
		stmt.Ast, err = parserInsert(s, &placeholder, &placeholderPos)
	case "DELETE":
		stmt.Ast, err = parserDelete(s, &placeholder, &placeholderPos)
	default:
		return Statement{}, errors.New("未能适配的SQL类型")
	}
	return stmt, err
}

// Marshal 将语法树生成新的SQL
func Marshal(stmt Statement) (string, error) {
	switch v := stmt.Ast.(type) {
	case Select:
		return marshalSelect(v)
	case Insert:
		return marshalInsert(v)
	case Update:
		return marshalUpdate(v)
	case Delete:
		return marshalDelete(v)
	default:
		return "", errors.New("不支持的语法树类型")
	}
	return "", errors.New("未知错误")
}

func (stmt *Statement) Type() string {
	switch stmt.Ast.(type) {
	case Select:
		return "SELECT"
	case Insert:
		return "INSERT"
	case Update:
		return "UPDATE"
	case Delete:
		return "DELETE"
	default:
		return ""
	}
}

// Params Value转Params类型
func (val *Value) Params() (Params, error) {
	switch v := val.Value.(type) {
	case Params:
		return v, nil
	default:
		return Params{}, errors.New("非参数类型")
	}
}

// Params 找出所有的SQL参数，会去重
func (stmt *Statement) Params() []Params {
	switch v := stmt.Ast.(type) {
	case Select:
		return getParamsBySelect(v)
	case Insert:
		return getParamsByInsert(v)
	case Update:
		return getParamsByUpdate(v)
	case Delete:
		return getParamsByDelete(v)
	default:
		return nil
	}
	return nil
}

func getParamsBySelect(sel Select) (pars []Params) {
	for _, item := range sel.Select {
		pars = append(pars, getParamsBySelectItem(item)...)
	}
	return pars
}

func getParamsBySelectItem(sel SelectItem) (pars []Params) {
	//字段
	pars = append(pars, getParamsBySelectField(sel.Field)...)
	//表
	pars = append(pars, getParamsBySelectTableList(sel.Table)...)
	//条件
	if sel.Where.Equation != nil {
		pars = append(pars, getParamsBySelectEquationList(sel.Where)...)
	}
	//分组
	if len(sel.Group) > 0 {
		for _, item := range sel.Group {
			pars = append(pars, getParamsBySelectValue(item)...)
		}
	}
	//分组条件
	if len(sel.Having.Equation) > 0 {
		pars = append(pars, getParamsBySelectEquationList(sel.Having)...)
	}
	//排序
	if sel.Order != nil {
		switch v := sel.Order.(type) {
		case OrderBy:
			if len(v.Value) == 0 {
				return nil
			}
			for _, item := range v.Value {
				pars = append(pars, getParamsBySelectValue(item)...)
			}
		case Function:
			pars = append(pars, getParamsBySelectFunction(v)...)
		}
	}
	return pars
}

func getParamsBySelectTableList(tables []SelectTable) (pars []Params) {
	if len(tables) == 0 {
		return nil
	}
	//每张表用逗号隔开
	for _, item := range tables {
		pars = append(pars, getParamsBySelectTable(item.Table)...)
		if item.JoinKey != "" {
			pars = append(pars, getParamsBySelectEquationList(item.JoinOn)...)
		}
	}
	return pars
}

func getParamsBySelectTable(tables interface{}) (pars []Params) {
	//表可能是字符串，也可能是子查询，子查询需要用括号括起
	switch v := tables.(type) {
	case Select:
		pars = append(pars, getParamsBySelect(v)...)
	case []SelectTable:
		pars = append(pars, getParamsBySelectTableList(v)...)
	case SelectTable:
		pars = append(pars, getParamsBySelectTable(v)...)
	}
	return pars
}

func getParamsBySelectField(flds []SelectField) (pars []Params) {
	for _, item := range flds {
		pars = append(pars, getParamsBySelectValue(item.Field)...)
	}
	return pars
}

func getParamsBySelectValue(value Value) (pars []Params) {
	switch v := value.Value.(type) {
	case Select:
		pars = append(pars, getParamsBySelect(v)...)
	case Function:
		pars = append(pars, getParamsBySelectFunction(v)...)
	case CaseWhen:
		pars = append(pars, getParamsBySelectCaseWhen(v)...)
	case Number:
		pars = append(pars, getParamsBySelectNumber(v)...)
	case Params:
		pars = append(pars, v)
	case Value:
		pars = append(pars, getParamsBySelectValue(v)...)
	case []Value:
		for _, item := range v {
			pars = append(pars, getParamsBySelectValue(item)...)
		}
	}
	return pars
}

func getParamsBySelectFunction(function Function) (pars []Params) {
	for _, item := range function.Params {
		pars = append(pars, getParamsBySelectValue(item)...)
	}
	return pars
}

func getParamsBySelectCaseWhen(caseWhen CaseWhen) (pars []Params) {
	if len(caseWhen.When) == 0 {
		return nil
	}
	for _, item := range caseWhen.When {
		pars = append(pars, getParamsBySelectCaseWhenItem(item)...)
	}
	if caseWhen.Else.Value != nil {
		pars = append(pars, getParamsBySelectValue(caseWhen.Else)...)
	}
	return pars
}

func getParamsBySelectCaseWhenItem(caseWhenItem CaseWhenItem) (pars []Params) {
	pars = append(pars, getParamsBySelectEquationList(caseWhenItem.Equation)...)
	pars = append(pars, getParamsBySelectValue(caseWhenItem.Value)...)
	return pars
}

func getParamsBySelectEquationList(eqList EquationList) (pars []Params) {
	if len(eqList.Equation) == 0 {
		return nil
	}
	for _, item := range eqList.Equation {
		//外层的不带括号，如果遇上了EquationList，需带上括号
		switch v := item.Equation.(type) {
		case EquationNorm:
			pars = append(pars, getParamsBySelectEquationNorm(v)...)
		case EquationOther:
			pars = append(pars, getParamsBySelectEquationOther(v)...)
		case EquationBetween:
			pars = append(pars, getParamsBySelectEquationBetween(v)...)
		case EquationList:
			pars = append(pars, getParamsBySelectEquationList(v)...)
		}
	}
	return pars
}

func getParamsBySelectEquationNorm(eq EquationNorm) (pars []Params) {
	if eq.Operator != "<" && eq.Operator != "<=" && eq.Operator != ">" && eq.Operator != ">=" && eq.Operator != "=" && eq.Operator != "!=" && eq.Operator != "<>" {
		return nil
	}
	pars = append(pars, getParamsBySelectValue(eq.Left)...)
	pars = append(pars, getParamsBySelectValue(eq.Right)...)
	return pars
}

func getParamsBySelectEquationOther(eq EquationOther) (pars []Params) {
	if eq.Operator != "IS NULL" && eq.Operator != "IS NOT NULL" && eq.Operator != "IN" && eq.Operator != "NOT IN" && eq.Operator != "EXIST" && eq.Operator != "NOT EXIST" && eq.Operator != "LIKE" && eq.Operator != "NOT LIKE" {
		return nil
	}
	pars = append(pars, getParamsBySelectValue(eq.Left)...)
	for _, item := range eq.Right {
		pars = append(pars, getParamsBySelectValue(item)...)
	}
	return pars
}

func getParamsBySelectEquationBetween(eq EquationBetween) (pars []Params) {
	pars = append(pars, getParamsBySelectValue(eq.Field)...)
	pars = append(pars, getParamsBySelectValue(eq.Left)...)
	pars = append(pars, getParamsBySelectValue(eq.Right)...)
	return pars
}

func getParamsBySelectNumber(num Number) (pars []Params) {
	if len(num.Number) == 0 {
		return nil
	}
	for _, item := range num.Number {
		pars = append(pars, getParamsBySelectValue(item.Value)...)
	}
	return pars
}

func getParamsByInsert(insert Insert) (pars []Params) {
	if insert.Table == "" {
		return nil
	}
	switch v := insert.Values.(type) {
	case []Value:
		for _, item := range v {
			pars = append(pars, getParamsBySelectValue(item)...)
		}
	case Select:
		pars = append(pars, getParamsBySelect(v)...)
	}
	return pars
}

func getParamsByUpdate(update Update) (pars []Params) {
	if len(update.Value) == 0 {
		return nil
	}
	if update.Table == "" {
		return nil
	}
	for _, item := range update.Value {
		pars = append(pars, getParamsBySelectValue(item.Value)...)
	}
	if len(update.Where.Equation) != 0 {
		pars = append(pars, getParamsBySelectEquationList(update.Where)...)
	}
	return pars
}

func getParamsByDelete(delete Delete) (pars []Params) {
	if delete.Table == "" {
		return nil
	}
	if len(delete.Where.Equation) != 0 {
		pars = append(pars, getParamsBySelectEquationList(delete.Where)...)
	}
	return pars
}

// RemoveParams 给参数去重
func RemoveParams(pars []Params) (ret []Params) {
	for _, par := range pars {
		bFind := false
		for _, item := range ret {
			if item.Name == par.Name {
				bFind = true
				break
			}
		}
		if !bFind {
			ret = append(ret, par)
		}
	}
	return ret
}

// FindParamsByString 给定一个SQL，提取里面有哪些参数，这些参数按照顺序排列
func FindParamsByString(s string) (pars []Params) {
	var placeholder []Placeholder
	var placeholderPos int
	s = strings.TrimSpace(s)
	//替换单引号括起的内容
	s = replacePlaceholder(s, `'(?:[^']|'')*'`, &placeholder, &placeholderPos)
	//替换双引号
	s = replacePlaceholder(s, "\".*?\"", &placeholder, &placeholderPos)
	//替换反单引号
	s = replacePlaceholder(s, "`.*?`", &placeholder, &placeholderPos)
	re := regexp.MustCompile(`:\w*`)
	strs := re.FindAllString(s, -1)
	for _, item := range strs {
		pars = append(pars, Params{
			Name: item,
		})
	}
	return pars
}

// RemoveParams 移除指定参数，如果这个参数的上层是Equation，那就要移除整个条件，如果是方法，则移除整个方法。
func (stmt *Statement) DeleteParams(pars []Params) {
	switch v := stmt.Ast.(type) {
	case Select:
		stmt.Ast = deleteParamsBySelect(v, pars)
	case Insert:
		stmt.Ast = deleteParamsByInsert(v, pars)
	case Update:
		stmt.Ast = deleteParamsByUpdate(v, pars)
	case Delete:
		stmt.Ast = deleteParamsByDelete(v, pars)
	}
}

func deleteParamsBySelect(sel Select, pars []Params) Select {
	for i := 0; i < len(sel.Select); i++ {
		deleteParamsBySelectItem(&sel.Select[i], pars)
	}
	return sel
}

func deleteParamsBySelectItem(sel *SelectItem, pars []Params) {
	//字段
	deleteParamsBySelectField(&sel.Field, pars)
	//表
	deleteParamsBySelectTableList(&sel.Table, pars)
	//条件
	if sel.Where.Equation != nil {
		sel.Where = deleteParamsBySelectEquationList(sel.Where, pars)
		//删除以后，第一个条件不能有连接符
		if len(sel.Where.Equation) != 0 {
			sel.Where.Equation[0].Connector = ""
		}
	}
	//分组
	if len(sel.Group) > 0 {
		var newGroup []Value
		for _, item := range sel.Group {
			item = deleteParamsBySelectValue(item, pars)
			if item.Value != nil {
				newGroup = append(newGroup, item)
			}
		}
		sel.Group = newGroup
	}
	//分组条件
	if len(sel.Having.Equation) > 0 {
		sel.Having = deleteParamsBySelectEquationList(sel.Having, pars)
	}
	//排序
	if sel.Order != nil {
		switch v := sel.Order.(type) {
		case OrderBy:
			var newOrder []Value
			for _, item := range v.Value {
				item = deleteParamsBySelectValue(item, pars)
				if item.Value != nil {
					newOrder = append(newOrder, item)
				}
			}
			sel.Order = OrderBy{Value: newOrder, Collation: v.Collation}
		case Function:
			sel.Order = deleteParamsBySelectFunction(v, pars)
		}
	}
}

func deleteParamsBySelectField(flds *[]SelectField, pars []Params) {
	for i := 0; i < len(*flds); i++ {
		(*flds)[i].Field = deleteParamsBySelectValue((*flds)[i].Field, pars)
	}
}

func deleteParamsBySelectValue(value Value, pars []Params) (val Value) {
	//返回空val，代表这里有参数被删除了，所以它的上一级就删除
	val = value
	switch v := value.Value.(type) {
	case Select:
		val.Value = deleteParamsBySelect(v, pars)
	case Function:
		val.Value = deleteParamsBySelectFunction(v, pars)
	case CaseWhen:
		val.Value = deleteParamsBySelectCaseWhen(v, pars)
	case Number:
		val.Value = deleteParamsBySelectNumber(v, pars)
	case Params:
		for _, item := range pars {
			if v.Name == item.Name {
				return Value{Value: nil}
			}
		}
	case Value:
		val.Value = deleteParamsBySelectValue(v, pars)
	case []Value:
		var bufVal []Value
		for _, item := range v {
			buf := deleteParamsBySelectValue(item, pars)
			if buf.Value != nil {
				bufVal = append(bufVal, buf)
			}
		}
		if len(bufVal) != 0 {
			val.Value = bufVal
		}
	}
	return val
}

func deleteParamsBySelectFunction(function Function, pars []Params) interface{} {
	for i := 0; i < len(function.Params); i++ {
		function.Params[i] = deleteParamsBySelectValue(function.Params[i], pars)
		if function.Params[i].Value == nil {
			return nil
		}
	}
	return function
}

func deleteParamsBySelectCaseWhen(caseWhen CaseWhen, pars []Params) interface{} {
	var cw CaseWhen
	for i := 0; i < len(caseWhen.When); i++ {
		caseWhen.When[i] = deleteParamsBySelectCaseWhenItem(caseWhen.When[i], pars)
		if caseWhen.When[i].Equation.Equation != nil && caseWhen.When[i].Value.Value != nil {
			cw.When = append(cw.When, caseWhen.When[i])
		}
	}
	if caseWhen.Else.Value != nil {
		cw.Else.Value = deleteParamsBySelectValue(caseWhen.Else, pars)
	}
	if len(cw.When) == 0 {
		return nil
	}
	return cw
}

func deleteParamsBySelectCaseWhenItem(caseWhenItem CaseWhenItem, pars []Params) CaseWhenItem {
	caseWhenItem.Equation = deleteParamsBySelectEquationList(caseWhenItem.Equation, pars)
	caseWhenItem.Value.Value = deleteParamsBySelectValue(caseWhenItem.Value, pars)
	return caseWhenItem
}

func deleteParamsBySelectEquationList(eqList EquationList, pars []Params) (ret EquationList) {
	for _, item := range eqList.Equation {
		//外层的不带括号，如果遇上了EquationList，需带上括号
		switch v := item.Equation.(type) {
		case EquationNorm:
			item.Equation = deleteParamsBySelectEquationNorm(v, pars)
		case EquationOther:
			item.Equation = deleteParamsBySelectEquationOther(v, pars)
		case EquationBetween:
			item.Equation = deleteParamsBySelectEquationBetween(v, pars)
		case EquationList:
			item.Equation = deleteParamsBySelectEquationList(v, pars)
			switch nv := item.Equation.(type) {
			case EquationList:
				if len(nv.Equation) == 0 {
					item.Equation = nil
				}
			}
		}
		if item.Equation != nil {
			ret.Equation = append(ret.Equation, item)
		}
	}
	return ret
}

func deleteParamsBySelectEquationNorm(eq EquationNorm, pars []Params) interface{} {
	eq.Left = deleteParamsBySelectValue(eq.Left, pars)
	eq.Right = deleteParamsBySelectValue(eq.Right, pars)
	if eq.Left.Value == nil || eq.Right.Value == nil {
		return nil
	}
	return eq
}

func deleteParamsBySelectEquationOther(eq EquationOther, pars []Params) interface{} {
	//对于other而言，如果是in、exist这种，只要还有参数，就不该直接删除这个条件。如果是like，只要like后面还接参数，也不该删除
	var ret EquationOther
	ret.Operator = eq.Operator
	ret.Left.Value = deleteParamsBySelectValue(eq.Left, pars)
	if ret.Left.Value == nil {
		return nil
	}
	for _, item := range eq.Right {
		val := deleteParamsBySelectValue(item, pars)
		if val.Value != nil {
			ret.Right = append(ret.Right, val)
		}
	}
	//if ret.Operator == "IN" || ret.Operator == "NOT IN" || ret.Operator == "EXIST" || ret.Operator == "NOT EXIST"{
	//	if len(ret.Right) > 0{
	//		return ret
	//	}
	//	return nil
	//}
	if len(ret.Right) == 0 {
		return nil
	}
	return ret
}

func deleteParamsBySelectEquationBetween(eq EquationBetween, pars []Params) interface{} {
	eq.Field = deleteParamsBySelectValue(eq.Field, pars)
	eq.Left = deleteParamsBySelectValue(eq.Left, pars)
	eq.Right = deleteParamsBySelectValue(eq.Right, pars)
	if eq.Field.Value == nil || eq.Left.Value == nil || eq.Right.Value == nil {
		return nil
	}
	return eq
}

func deleteParamsBySelectNumber(num Number, pars []Params) interface{} {
	var ret Number
	for _, item := range num.Number {
		item.Value.Value = deleteParamsBySelectValue(item.Value, pars)
		if item.Value.Value != nil {
			ret.Number = append(ret.Number, item)
		}
	}
	if len(ret.Number) == 0 {
		return nil
	}
	return ret
}

func deleteParamsBySelectTableList(tables *[]SelectTable, pars []Params) {
	var newTabs []SelectTable
	for i := 0; i < len(*tables); i++ {
		(*tables)[i].Table = deleteParamsBySelectTable((*tables)[i].Table, pars)
		if (*tables)[i].JoinKey != "" {
			(*tables)[i].JoinOn = deleteParamsBySelectEquationList((*tables)[i].JoinOn, pars)
			if len((*tables)[i].JoinOn.Equation) == 0 {
				(*tables)[i].JoinKey = ""
			}
		}
		if (*tables)[i].Table != nil {
			newTabs = append(newTabs, (*tables)[i])
		}
	}
	tables = &newTabs
}

func deleteParamsBySelectTable(tables interface{}, pars []Params) interface{} {
	ret := tables
	//表可能是字符串，也可能是子查询，子查询需要用括号括起
	switch v := tables.(type) {
	case Select:
		return deleteParamsBySelect(v, pars)
	case []SelectTable:
		deleteParamsBySelectTableList(&v, pars)
		return v
		//case SelectTable:
		//	return deleteParamsBySelectTable(v, pars)
		//	return ret
	}
	return ret
}

func deleteParamsByInsert(insert Insert, pars []Params) Insert {
	switch v := insert.Values.(type) {
	case []Value:
		var newVal []Value
		for _, item := range v {
			item.Value = deleteParamsBySelectValue(item, pars)
			if item.Value != nil {
				newVal = append(newVal, item)
			}
		}
		insert.Values = newVal
	case Select:
		insert.Values = deleteParamsBySelect(v, pars)
	}
	return insert
}

func deleteParamsByUpdate(update Update, pars []Params) Update {
	var newValues []UpdateValueItem
	for i := 0; i < len(update.Value); i++ {
		update.Value[i].Value = deleteParamsBySelectValue(update.Value[i].Value, pars)
		if update.Value[i].Value.Value != nil {
			newValues = append(newValues, update.Value[i])
		}
	}
	update.Value = newValues
	if len(update.Where.Equation) != 0 {
		update.Where = deleteParamsBySelectEquationList(update.Where, pars)
	}
	return update
}

func deleteParamsByDelete(delete Delete, pars []Params) Delete {
	if len(delete.Where.Equation) != 0 {
		delete.Where = deleteParamsBySelectEquationList(delete.Where, pars)
	}
	return delete
}

// ExpandParams 给参数扩展参数，在IN、NOT IN里面的参数，如果传递的是数组，则需要对参数进行扩展，扩展的个数是count
func (stmt *Statement) ExpandParams(params Params, count int) {
	if count <= 0 {
		return
	}
	switch v := stmt.Ast.(type) {
	case Select:
		stmt.Ast = expandParamsBySelect(v, params, count)
	case Insert:
		stmt.Ast = expandParamsByInsert(v, params, count)
	case Update:
		stmt.Ast = expandParamsByUpdate(v, params, count)
	case Delete:
		stmt.Ast = expandParamsByDelete(v, params, count)
	}
}

func expandParamsBySelect(sel Select, params Params, count int) Select {
	for i := 0; i < len(sel.Select); i++ {
		expandParamsBySelectItem(&sel.Select[i], params, count)
	}
	return sel
}

func expandParamsBySelectItem(sel *SelectItem, params Params, count int) {
	//字段
	expandParamsBySelectField(&sel.Field, params, count)
	//表
	expandParamsBySelectTableList(&sel.Table, params, count)
	//条件
	if sel.Where.Equation != nil {
		sel.Where = expandParamsBySelectEquationList(sel.Where, params, count)
	}
	//分组
	if len(sel.Group) > 0 {
		var newGroup []Value
		for _, item := range sel.Group {
			val := expandParamsBySelectValue(item, params, count)
			if val.Value != nil {
				newGroup = append(newGroup, val)
			} else {
				newGroup = append(newGroup, item)
			}
		}
		sel.Group = newGroup
	}
	//分组条件
	if len(sel.Having.Equation) > 0 {
		sel.Having = expandParamsBySelectEquationList(sel.Having, params, count)
	}
	//排序
	if sel.Order != nil {
		switch v := sel.Order.(type) {
		case OrderBy:
			var newOrder []Value
			for _, item := range v.Value {
				val := expandParamsBySelectValue(item, params, count)
				if val.Value != nil {
					newOrder = append(newOrder, val)
				} else {
					newOrder = append(newOrder, item)
				}
			}
			sel.Order = OrderBy{Value: newOrder, Collation: v.Collation}
		case Function:
			sel.Order = expandParamsBySelectFunction(v, params, count)
		}
	}
}

func expandParamsBySelectField(flds *[]SelectField, params Params, count int) {
	for i := 0; i < len(*flds); i++ {
		(*flds)[i].Field = expandParamsBySelectValue((*flds)[i].Field, params, count)
	}
}

func expandParamsBySelectValue(value Value, params Params, count int) (val Value) {
	val = value
	switch v := value.Value.(type) {
	case Select:
		val.Value = expandParamsBySelect(v, params, count)
	case Function:
		val.Value = expandParamsBySelectFunction(v, params, count)
	case CaseWhen:
		val.Value = expandParamsBySelectCaseWhen(v, params, count)
	case Number:
		val.Value = expandParamsBySelectNumber(v, params, count)
	case Params:
		if v.Name == params.Name {
			return Value{Value: nil}
		}
	case Value:
		val.Value = expandParamsBySelectValue(v, params, count)
	case []Value:
		var bufVal []Value
		for _, item := range v {
			buf := expandParamsBySelectValue(item, params, count)
			if buf.Value == nil {
				buf = item
			}
			bufVal = append(bufVal, buf)
		}
		val.Value = bufVal
	}
	return val
}

func expandParamsBySelectFunction(function Function, params Params, count int) interface{} {
	for i := 0; i < len(function.Params); i++ {
		val := expandParamsBySelectValue(function.Params[i], params, count)
		if val.Value != nil {
			function.Params[i] = val
		}
	}
	return function
}

func expandParamsBySelectCaseWhen(caseWhen CaseWhen, params Params, count int) interface{} {
	var cw CaseWhen
	for i := 0; i < len(caseWhen.When); i++ {
		val := expandParamsBySelectCaseWhenItem(caseWhen.When[i], params, count)
		if val.Equation.Equation != nil && val.Value.Value != nil {
			val = caseWhen.When[i]
		}
		cw.When = append(cw.When, val)
	}
	if caseWhen.Else.Value != nil {
		cw.Else.Value = expandParamsBySelectValue(caseWhen.Else, params, count)
		if cw.Else.Value == nil {
			cw.Else = caseWhen.Else
		}
	}
	return cw
}

func expandParamsBySelectCaseWhenItem(caseWhenItem CaseWhenItem, params Params, count int) CaseWhenItem {
	ret := caseWhenItem
	ret.Equation = expandParamsBySelectEquationList(caseWhenItem.Equation, params, count)
	caseWhenItem.Value.Value = expandParamsBySelectValue(caseWhenItem.Value, params, count)
	if caseWhenItem.Value.Value != nil {
		ret.Value = caseWhenItem.Value
	}
	return ret
}

func expandParamsBySelectEquationList(eqList EquationList, params Params, count int) (ret EquationList) {
	for _, item := range eqList.Equation {
		//外层的不带括号，如果遇上了EquationList，需带上括号
		switch v := item.Equation.(type) {
		case EquationNorm:
			item.Equation = expandParamsBySelectEquationNorm(v, params, count)
		case EquationOther:
			item.Equation = expandParamsBySelectEquationOther(v, params, count)
		case EquationBetween:
			item.Equation = expandParamsBySelectEquationBetween(v, params, count)
		case EquationList:
			item.Equation = expandParamsBySelectEquationList(v, params, count)
			switch nv := item.Equation.(type) {
			case EquationList:
				if len(nv.Equation) == 0 {
					item.Equation = nil
				}
			}
		}
		if item.Equation != nil {
			ret.Equation = append(ret.Equation, item)
		}
	}
	return ret
}

func expandParamsBySelectEquationNorm(eq EquationNorm, params Params, count int) interface{} {
	var ret EquationNorm
	ret.Left = expandParamsBySelectValue(eq.Left, params, count)
	ret.Right = expandParamsBySelectValue(eq.Right, params, count)
	if ret.Left.Value == nil {
		ret.Left = eq.Left
	}
	if ret.Right.Value == nil {
		ret.Right = eq.Right
	}
	ret.Operator = eq.Operator
	return ret
}

func expandParamsBySelectEquationOther(eq EquationOther, params Params, count int) interface{} {
	//对于other而言，如果是in、exist这种，只要还有参数，就不该直接删除这个条件。如果是like，只要like后面还接参数，也不该删除
	var ret EquationOther
	ret.Operator = eq.Operator
	ret.Left.Value = expandParamsBySelectValue(eq.Left, params, count)
	if ret.Left.Value == nil {
		ret.Left = eq.Left
	}
	for _, item := range eq.Right {
		val := expandParamsBySelectValue(item, params, count)
		if val.Value == nil {
			//说明是需要扩展的参数
			par, _ := item.Params()
			for i := 0; i < count; i++ {
				newPar := par
				newPar.Name = par.Name + strconv.Itoa(i)
				ret.Right = append(ret.Right, Value{Value: newPar})
			}
		} else {
			ret.Right = append(ret.Right, item)
		}
	}
	return ret
}

func expandParamsBySelectEquationBetween(eq EquationBetween, params Params, count int) interface{} {
	var ret EquationBetween
	ret.Field = expandParamsBySelectValue(eq.Field, params, count)
	ret.Left = expandParamsBySelectValue(eq.Left, params, count)
	ret.Right = expandParamsBySelectValue(eq.Right, params, count)
	if ret.Field.Value == nil {
		ret.Field = eq.Field
	}
	if ret.Left.Value == nil {
		ret.Left = eq.Left
	}
	if ret.Right.Value == nil {
		ret.Right = eq.Right
	}
	return ret
}

func expandParamsBySelectNumber(num Number, params Params, count int) interface{} {
	var ret Number
	for _, item := range num.Number {
		val := expandParamsBySelectValue(item.Value, params, count)
		if item.Value.Value == nil {
			ret.Number = append(ret.Number, item)
		} else {
			item.Value.Value = val
			ret.Number = append(ret.Number, item)
		}
	}
	return ret
}

func expandParamsBySelectTableList(tables *[]SelectTable, params Params, count int) {
	var newTabs []SelectTable
	for i := 0; i < len(*tables); i++ {
		(*tables)[i].Table = expandParamsBySelectTable((*tables)[i].Table, params, count)
		if (*tables)[i].JoinKey != "" {
			(*tables)[i].JoinOn = expandParamsBySelectEquationList((*tables)[i].JoinOn, params, count)
			if len((*tables)[i].JoinOn.Equation) == 0 {
				(*tables)[i].JoinKey = ""
			}
		}
		if (*tables)[i].Table != nil {
			newTabs = append(newTabs, (*tables)[i])
		}
	}
	tables = &newTabs
}

func expandParamsBySelectTable(tables interface{}, params Params, count int) interface{} {
	ret := tables
	//表可能是字符串，也可能是子查询，子查询需要用括号括起
	switch v := tables.(type) {
	case Select:
		return expandParamsBySelect(v, params, count)
	case []SelectTable:
		expandParamsBySelectTableList(&v, params, count)
		return v
	case SelectTable:
		return expandParamsBySelectTable(v, params, count)
	}
	return ret
}

func expandParamsByInsert(insert Insert, params Params, count int) Insert {
	switch v := insert.Values.(type) {
	case []Value:
		var newVal []Value
		for _, item := range v {
			val := expandParamsBySelectValue(item, params, count)
			if val.Value != nil {
				newVal = append(newVal, val)
			} else {
				newVal = append(newVal, item)
			}
		}
		insert.Values = newVal
	case Select:
		insert.Values = expandParamsBySelect(v, params, count)
	}
	return insert
}

func expandParamsByUpdate(update Update, params Params, count int) Update {
	var newValues []UpdateValueItem
	for i := 0; i < len(update.Value); i++ {
		val := expandParamsBySelectValue(update.Value[i].Value, params, count)
		if val.Value != nil {
			update.Value[i].Value = val
		}
		newValues = append(newValues, update.Value[i])
	}
	update.Value = newValues
	if len(update.Where.Equation) != 0 {
		update.Where = expandParamsBySelectEquationList(update.Where, params, count)
	}
	return update
}

func expandParamsByDelete(delete Delete, params Params, count int) Delete {
	if len(delete.Where.Equation) != 0 {
		delete.Where = expandParamsBySelectEquationList(delete.Where, params, count)
	}
	return delete
}
