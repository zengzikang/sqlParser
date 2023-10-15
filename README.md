# sqlParser
基于oracle语法的SQL序列化和反序列化的库，目前仅支持增删改查

SQL解析器
=================================
SQL解析器可对Oracle语法的SQL进行解析，并生成语法树，它还可以将SQL语法树生成Oracle语法的SQL。后续会稍加改进，让它同时支持Oracle和Mysql。
有了它，你可以对SQL进行基本的语法检查，也可通过它实现一些自动化的工作。
## 结构体大全
* **Value**
````azure
/*SQL的值，它可以是
1. 子查询：Select
2. 函数：Function
3. CASE WHEN表达式：CaseWhen
4. 字符串：string
5. 数字（通常有加减乘除运算才会定义成数字）：Number
6. 参数：Params
7. 被双竖线连接的值组合：[]Value
*/
type Value struct {
	Value	interface{}
}
````
* **Equation**
```azure
/*一个比较式
  它可以是 “左值 符号（>、>=、<、<=、=） 右值”
  “Between 值 and 值”
  “值 IS (NOT)? NULL”
  “值 (NOT)? LIKE 值”
  “值 (NOT)? IN(值...)”
  “值 (NOT)? EXIST(值...)”
  它们之间需使用AND/OR连接
  在Select语句中，Case when的条件、where的条件、having的条件、join on中的条件都是由它构成
*/
type Equation struct {
	Equation interface{}	        //它可以是EquationNorm、EquationOther、EquationBetween, EquationList
	Connector string		//连接符只能是AND/OR两种
}
```
* **EquationNorm**
```azure
/*常规比较式，即左值 符号 右值
  例：RECNO=1
    ROWNUM<10
*/
type EquationNorm struct {
	Left		Value
	Right		Value
	Operator	string
}
```
* **EquationBetween**
```azure
/*Between and表达式*/
type EquationBetween struct {
	Left		Value
	Right		Value
	Field		Value
}
```
* **EquationOther**
```azure
/*其他比较式*/
type EquationOther struct {
	Left		Value
	Operator	string			//它可以是IS (NOT)?   (NOT)? (LIKE)|(IN)|(EXIST)
	Right		[]Value			//如果是NULL的，直接就是字符串NULL，其他的则需要括号表示
}
```
* **EquationList**
```azure
/*条件列表，像Where条件等都是由它构成的。当它出现在单例条件中，说明它是被括号括起的条件组*/
type EquationList struct {
	Equation	[]Equation
	Connector	string
}
```
* **Function**
```azure
/*SQL中的函数，函数由函数名，外加n个Value值组成，各参数值用逗号隔开*/
type Function struct {
	Name	string
	Params	[]Value
}
```
* **CaseWhen**
```azure
/*case when表达式：它有两种表达方式
1. case 值 when 值 then 值 else 值 end;
2. case when 条件 then 值 else 值 end;
不管是哪种表达式，实际上最终都指向了第二种，第一种翻译成第二种就是
case when 值=值 then 值 else 值 end;*/
type CaseWhen struct{
	When	[]CaseWhenItem
	Else	Value
}
```
* **CaseWhenItem**
```azure
/*case when表达式中的单项when
  它包含when之后的条件列，以及then的值*/
type CaseWhenItem struct {
	Equation	EquationList
	Value		Value
}
```
* **Number**
```azure
/*数字：通常出现了加减乘除运算才会是这个结构*/
type Number struct {
	Number		[]NumberItem
}
```
* **NumberItem**
```azure
/*数字项：数字的最小单位，它和上一个数字的运算符号保存在Operator中*/
type NumberItem struct {
	Value		Value
	Operator	string
}
```
* **Params**
```azure
/*Oracle形式的参数，Name就是参数名称*/
type Params struct {
	Name		string
}
```
* **OrderBy**
```azure
/*查询排序，Order By的形式*/
type OrderBy struct {
	Value		[]Value         //值列表，用逗号隔开的
	Collation	string          //排序方式：ASC 正序；AESC 倒序
}
```
* **Statement**
```azure
/*SQL语法树*/
type Statement struct {
	Ast		interface{}         //它有：Select、Update、Insert、Delete
}
```
* **Select**
```azure
/*查询SQL的语法树*/
type Select struct {
	Select	[]SelectItem
}
```
* **SelectItem**
```azure
/*单查询的语法树，即由SELECT 值列表 FROM 表列表 JOIN 表 ON GROUP BY 值列表 HAVING 条件列表 ORDER 排序 基本SQL组成的
    一个完整的查询SQL，除了单查询外，还可能使用了集合关键词进行连接*/
type SelectItem struct {
	Field		[]SelectField
	Table		[]SelectTable
	Where		EquationList
	Group 		[]Value
	Having		EquationList
	Order		interface{}			//它可以是[]Value(Order By)、Function(Order Decode)
	Aggregate	string				//集合关键词：union、union all、minus、intersect
}
```
* **SelectField**
```azure
/*查询语句的查询字段，查询的字段由：”值 别名“组成*/
type SelectField struct {
	Field		Value
	Alias		string
}
```
* **SelectTable**
```azure
/*查询语句的表*/
type SelectTable struct {
	Table		interface{}		//它可以是子查询，也可以是字符串
	Alias		string			//别名
	JoinKey		string			//如果这张表是join前面的表，则会有关键词，它可以是JOIN、LEFT JOIN、RIGHT JOIN、INNER JOIN
	JoinOn		EquationList	        //一个条件列，它可以被括号括起来
}
```
* **Placeholder**
```azure
/*为了解析SQL使结构完整，把一些字符串用占位符替代，这些被替代的字符串全部保存在这个结构体中，该结构体不会在SQL语法树中体现*/
type Placeholder struct {
	Value		string
	Name		string			//所有占位符的名称都应该是$序号，例$000001
}
```

----------------------------------------------------------
## 方法大全
* **removeExtraSpaces**
```azure
/*清除多余的空格：它会把多余的空格、制表符、换行符等都替换成一个空格*/
func removeExtraSpaces(s string) string
```
* **placeholderByString**
```azure
/*将一段SQL中可以替换成占位符的字符串替换成占位符
  1. 被单引号括起的部分
  2. 被双引号括起的部分
  3. 被反单引号括起的部分
  4. Oracle形式的参数，例  :RECNO
  5. 被小括号括起的部分：它会从里到外依次替换。
    例，(2 * (3 + 5))
    它会先替换里面的括号内容，替换后就变成 (2 * $000000)
    然后他会继续往外找，这时它的整体会替换为 $000001
  */
func placeholderByString(s string, placeholder *[]Placeholder, placeholderPos *int) (string, error)
```
* **replacePlaceholder**
```azure
/*用正则表达式替换占位符
  在将SQL替换成占位符的过程中，有一些可以通过正则表达式匹配的字符串替换成占位符的，可以用该方法。
  例placeholderByString方法中的前4项都是用该方法实现的
*/
func replacePlaceholder(s string, regStr string, placeholder *[]Placeholder, placeholderPos *int) string
```
* **replaceParenthesis**
```azure
/*把被小括号括起的部分从里到外全部替换成占位符*/
func replaceParenthesis(s string, placeholder *[]Placeholder, placeholderPos *int) (string, error)
```
* **getPlaceholder**
```azure
/*传入一个字符串，返回被占位符替代还原后的字符串和占位符列表
  例：$000001
  解析后：(2 * $000000)
  总结：它只会还原一层，如果遇到占位符嵌套的，需要再次进行还原
*/
func getPlaceholder(s string, placeholder *[]Placeholder, placeholderPos *int) (retStr string, retPlace []Placeholder, err error)
```
* **getSqlType**
```azure
/*获取SQL的语法类型
  说白了就是获取第一个空格之前的字符串*/
func getSqlType(s string, placeholder *[]Placeholder, placeholderPos *int) (string, error)
```
* **trimLR**
```azure
/*同时满足左右两边都有的情况下才会去掉
  去除字符串首尾两端特定的字符串，只有两端都符合要求才会去除
  大多情况下会用来去除被小括号括起的字符串，即：去除左右括号*/
func trimLR(s, l, r string) string
```
* **splitSQLAggregate**
```azure
/*如果一个SQL中存在集合关键词(union、union all、minus、intersect)，则需要拆分成多个单条SQL*/
func splitSQLAggregate(s string) ([]string, []string)
```
* **parserSelect**
```azure
/*对一个完整的查询SQL进行解析，返回Select的语法树*/
func parserSelect(s string, placeholder *[]Placeholder, placeholderPos *int) (sel Select, err error)
```
* **parserSelectItem**
```azure
/*对单查询的SQL进行解析，返回单查询的语法树*/
func parserSelectItem(s string, placeholder *[]Placeholder, placeholderPos *int)(sel SelectItem, err error)
```
* **getTable**
```azure
/*传入被查询的表，返回表的结构体，即：表 别名*/
func getTable(s string, placeholder *[]Placeholder, placeholderPos *int)(table SelectTable, err error)
```
* **splitJoin**
```azure
/*提取出join的部分，传入的字符串应确认有JOIN的存在。返回的应该是JOIN前边的表、JOIN本身、ON后面的部分*/
func splitJoin(s string) (rets []JoinString)
    
type JoinString struct {
	Table	string
	Keyword	string
	On	string
}
```
* **getSelectOrder**
```azure
/*解析Order排序*/
func getSelectOrder(s string, placeholder *[]Placeholder, placeholderPos *int) (order interface{}, err error)
```
* **getSelectGroup**
```azure
/*解析分组*/
func getSelectGroup(s string, placeholder *[]Placeholder, placeholderPos *int) (groups []Value , err error)
```
* **getSelectTable**
```azure
/*解析查询语句的表，里面包含了JOIN部分*/
func getSelectTable(s string, placeholder *[]Placeholder, placeholderPos *int) (tables []SelectTable, err error)
```
* **getSelectField**
```azure
/*解析查询语句的字段部分*/
func getSelectField(s string, placeholder *[]Placeholder, placeholderPos *int) (fields []SelectField, err error)
```
* **getValue**
```azure
/*解析SQL值的部分*/
func getValue(s string, placeholder *[]Placeholder, placeholderPos *int)(value Value, err error)
```
* **getFunction**
```azure
/*解析函数的部分*/
func getFunction(name, params string, placeholder *[]Placeholder, placeholderPos *int)(f Function, err error)
```
* **replaceBetweenItem**
```azure
/*将between and表达式替换成$BETWEEN*/
func replaceBetweenItem(s string) (newStr string, betStr string, err error)
func replaceBetween(s string) (newStr string, betStrs []string, err error)
```
* **getEquationList**
```azure
/*解析条件部分*/
func getEquationList(s string, placeholder *[]Placeholder, placeholderPos *int)(list EquationList, err error)
```
* **getCaseWhen**
```azure
/*解析Case when表达式*/
func getCaseWhen(s string, placeholder *[]Placeholder, placeholderPos *int) (cas CaseWhen, err error)
```
* **getNumberItemKeyword**
```azure
/*将字符串按照+-*/分割*/
func getNumberItemKeyword(s string) (items []NumberItemKeyword)
    
type NumberItemKeyword struct {
	Value		string
	Operator	string
}
```
* **splitSqlByKeywordForSelect**
```azure
/*将一个单查询按关键词进行分割*/
func splitSqlByKeywordForSelect(s string) (sel SelectKeyword, err error)
func FindKeyPos(s string, keys []KeyPos) []KeyPos

type SelectKeyword struct {
    Select		string
    From		string
    Where		string
    Group		string
    Having		string
    Order		string
}

type KeyPos struct {
    Key			string
    Start		int
    End			int
}
```
* **Unmarshal**
```azure
/*将SQL解析成语法树*/
func Unmarshal(s string)(stmt Statement, err error)
```
